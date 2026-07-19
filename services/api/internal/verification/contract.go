package verification

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
)

const ContractVersion = "paritylab.verification.v1"

type Fault string

const (
	FaultNone      Fault = "none"
	FaultDuplicate Fault = "duplicate"
	FaultReorder   Fault = "reorder"
	FaultTimeout   Fault = "timeout"
	FaultTamper    Fault = "tamper"
)

type Delivery struct {
	Version     string `json:"version"`
	EventID     string `json:"event_id"`
	EffectID    string `json:"effect_id"`
	RunID       string `json:"run_id"`
	Sequence    int    `json:"sequence"`
	PayloadHash string `json:"payload_hash"`
	Signature   string `json:"signature"`
}

type Receipt struct {
	Accepted        bool
	Duplicate       bool
	SignatureValid  bool
	BusinessEffects int
}

type Result struct {
	Fault           Fault
	Attempts        int
	Accepted        int
	Duplicates      int
	Rejected        int
	BusinessEffects int
}

type Target interface {
	Deliver(context.Context, Delivery) (Receipt, error)
}

type Signer struct{ secret []byte }

func NewSigner(secret string) (*Signer, error) {
	if len(secret) < 16 {
		return nil, errors.New("verification signing secret must contain at least 16 bytes")
	}
	return &Signer{secret: []byte(secret)}, nil
}

func (s *Signer) Sign(delivery Delivery) string {
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write([]byte(signingInput(delivery)))
	return hex.EncodeToString(mac.Sum(nil))
}

func (s *Signer) Verify(delivery Delivery) bool {
	expected := s.Sign(delivery)
	provided, err := hex.DecodeString(delivery.Signature)
	if err != nil {
		return false
	}
	want, _ := hex.DecodeString(expected)
	return hmac.Equal(provided, want)
}

func signingInput(delivery Delivery) string {
	return fmt.Sprintf("%s\n%s\n%s\n%s\n%d\n%s", delivery.Version, delivery.EventID, delivery.EffectID, delivery.RunID, delivery.Sequence, delivery.PayloadHash)
}

// ReferenceMerchant is a bundled target that models an order mutation with
// signature verification, effect-level idempotency, and monotonic sequence state.
type ReferenceMerchant struct {
	mu       sync.Mutex
	signer   *Signer
	effects  map[string]struct{}
	sequence map[string]int
}

func NewReferenceMerchant(signer *Signer) *ReferenceMerchant {
	return &ReferenceMerchant{signer: signer, effects: make(map[string]struct{}), sequence: make(map[string]int)}
}

func (m *ReferenceMerchant) Deliver(_ context.Context, delivery Delivery) (Receipt, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if delivery.Version != ContractVersion || !m.signer.Verify(delivery) {
		return Receipt{Accepted: false, SignatureValid: false, BusinessEffects: len(m.effects)}, nil
	}
	if previous := m.sequence[delivery.EffectID]; delivery.Sequence < previous {
		return Receipt{Accepted: true, Duplicate: true, SignatureValid: true, BusinessEffects: len(m.effects)}, nil
	}
	m.sequence[delivery.EffectID] = delivery.Sequence
	if _, exists := m.effects[delivery.EffectID]; exists {
		return Receipt{Accepted: true, Duplicate: true, SignatureValid: true, BusinessEffects: len(m.effects)}, nil
	}
	m.effects[delivery.EffectID] = struct{}{}
	return Receipt{Accepted: true, SignatureValid: true, BusinessEffects: len(m.effects)}, nil
}

type Relay struct {
	signer *Signer
	target Target
}

func NewRelay(signer *Signer, target Target) *Relay { return &Relay{signer: signer, target: target} }

func (r *Relay) Execute(ctx context.Context, base Delivery, fault Fault) (Result, error) {
	base.Version = ContractVersion
	base.Signature = r.signer.Sign(base)
	deliveries := []Delivery{base}
	result := Result{Fault: fault}
	switch fault {
	case FaultNone:
	case FaultDuplicate:
		deliveries = append(deliveries, base)
	case FaultReorder:
		newer := base
		newer.EventID += "_newer"
		newer.Sequence++
		newer.Signature = r.signer.Sign(newer)
		deliveries = []Delivery{newer, base}
	case FaultTimeout:
		// The first attempt is intentionally not delivered; the retry uses the
		// exact signed envelope and exercises the same idempotent target path.
		result.Attempts++
	case FaultTamper:
		deliveries[0].PayloadHash = stringsOfZero(len(deliveries[0].PayloadHash))
	default:
		return Result{}, fmt.Errorf("unsupported verification fault %q", fault)
	}
	for _, delivery := range deliveries {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		result.Attempts++
		receipt, err := r.target.Deliver(ctx, delivery)
		if err != nil {
			return result, err
		}
		result.BusinessEffects = receipt.BusinessEffects
		if receipt.Accepted {
			result.Accepted++
		} else {
			result.Rejected++
		}
		if receipt.Duplicate {
			result.Duplicates++
		}
	}
	return result, nil
}

func stringsOfZero(length int) string {
	value := make([]byte, length)
	for index := range value {
		value[index] = '0'
	}
	return string(value)
}
