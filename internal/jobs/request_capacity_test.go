package jobs

import "testing"

func TestRequestRunnerFinalStressCapacityRequiresProvisionedTenantQueue(t *testing.T) {
	defaults := &RequestRunner{limits: Defaults()}
	if defaults.SupportsTenantConcurrency(128) || defaults.SupportsTenantConcurrency(0) || (*RequestRunner)(nil).SupportsTenantConcurrency(128) {
		t.Fatal("default or invalid queue claimed the final stress concurrency")
	}
	provisioned := Defaults()
	provisioned.Workers = 32
	provisioned.GlobalConcurrency = 128
	provisioned.TenantConcurrency = 128
	if provisioned.Validate() != nil || !(&RequestRunner{limits: provisioned}).SupportsTenantConcurrency(128) || (&RequestRunner{limits: provisioned}).SupportsTenantConcurrency(129) {
		t.Fatal("bounded provisioned queue did not match its declared release capacity")
	}
	provisioned.MaxPendingPerTenant = 127
	if (&RequestRunner{limits: provisioned}).SupportsTenantConcurrency(128) {
		t.Fatal("tenant pending budget was mistaken for final stress capacity")
	}
}
