package worker

import (
	"context"

	"github.com/ayushkumarsingh/paritylab/services/api/internal/engine"
	"github.com/ayushkumarsingh/paritylab/services/api/internal/verification"
)

// RepositoryMerchant is the bundled verification target backed by the same
// durable repository as the worker. Its effect-level idempotency survives a
// worker process restart.
type RepositoryMerchant struct {
	repo   engine.Repository
	signer *verification.Signer
}

func NewRepositoryMerchant(repo engine.Repository, signer *verification.Signer) *RepositoryMerchant {
	return &RepositoryMerchant{repo: repo, signer: signer}
}

func (m *RepositoryMerchant) Deliver(ctx context.Context, delivery verification.Delivery) (verification.Receipt, error) {
	if delivery.Version != verification.ContractVersion || !m.signer.Verify(delivery) {
		return verification.Receipt{Accepted: false, SignatureValid: false}, nil
	}
	duplicate, err := m.repo.ApplyReferenceMerchantEffect(ctx, delivery.EffectID, delivery.Sequence)
	if err != nil {
		return verification.Receipt{}, err
	}
	return verification.Receipt{
		Accepted: true, Duplicate: duplicate, SignatureValid: true, BusinessEffects: 1,
	}, nil
}
