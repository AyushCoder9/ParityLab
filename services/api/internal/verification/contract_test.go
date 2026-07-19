package verification

import (
	"context"
	"testing"
)

func TestControlledFaultRelay(t *testing.T) {
	t.Parallel()
	for _, fault := range []Fault{FaultNone, FaultDuplicate, FaultReorder, FaultTimeout, FaultTamper} {
		t.Run(string(fault), func(t *testing.T) {
			signer, _ := NewSigner("reference-merchant-test-secret")
			merchant := NewReferenceMerchant(signer)
			relay := NewRelay(signer, merchant)
			result, err := relay.Execute(context.Background(), Delivery{
				EventID: "evt_test", EffectID: "order_test", RunID: "run_000004",
				Sequence: 1, PayloadHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			}, fault)
			if err != nil {
				t.Fatal(err)
			}
			if fault == FaultTamper {
				if result.Rejected != 1 || result.BusinessEffects != 0 {
					t.Fatalf("tamper result=%+v", result)
				}
				return
			}
			if result.BusinessEffects != 1 {
				t.Fatalf("fault=%s result=%+v", fault, result)
			}
			if (fault == FaultDuplicate || fault == FaultReorder) && result.Duplicates != 1 {
				t.Fatalf("fault=%s expected duplicate result=%+v", fault, result)
			}
		})
	}
}
