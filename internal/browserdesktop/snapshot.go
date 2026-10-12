// Package browserdesktop defines the allowlisted, passive Desktop observation.
package browserdesktop

// ConnectorHealthNotObserved explicitly excludes live reachability/authentication claims.
const ConnectorHealthNotObserved = "not_observed"

// SharedPoolBudget is bookkeeping only, not live browser or consent evidence.
type SharedPoolBudget struct {
	Capacity  int `json:"capacity"`
	Occupied  int `json:"occupied"`
	Available int `json:"available"`
	Claimed   int `json:"claimed"`
	Uncertain int `json:"uncertain"`
}

type Snapshot struct {
	AuthenticatedTransportOwners int               `json:"authenticatedTransportOwners"` // Authorized endpoint access only.
	ManagedPoolMode              string            `json:"managedPoolMode"`
	SharedPoolBudget             *SharedPoolBudget `json:"sharedPoolBudget,omitempty"`

	ConfiguredConnectors                int    `json:"configuredConnectors"`
	ConfiguredAuthenticatedEdgeProfiles int    `json:"configuredAuthenticatedEdgeProfiles"`
	ConfiguredRequiredExternalPolicies  int    `json:"configuredRequiredExternalPolicies"`
	ConnectorHealth                     string `json:"connectorHealth"`
	ManagedLeases                       int    `json:"managedLeases"`
	RequiredExternalLeases              int    `json:"requiredExternalLeases"`
	ExplicitExternalLeases              int    `json:"explicitExternalLeases"`
	ObservedAt                          string `json:"observedAt"`
	Availability                        string `json:"availability"`
	State                               string `json:"state"`
	Stale                               bool   `json:"stale"`
	BrowserEnabled                      bool   `json:"browserEnabled"`
	ACPEnabled                          bool   `json:"acpEnabled"`
	CompanyRequiredEdgePolicies         int    `json:"companyRequiredEdgePolicies"`
	Owners                              int    `json:"owners"`
	Leases                              int    `json:"leases"`
	ActiveLeases                        int    `json:"activeLeases"`
	ExpiredLeases                       int    `json:"expiredLeases"`
	ReleasingLeases                     int    `json:"releasingLeases"`
	FailedLeases                        int    `json:"failedLeases"`
	UnownedLeases                       int    `json:"unownedLeases"`
	Workers                             int    `json:"workers"`
	ReadyWorkers                        int    `json:"readyWorkers"`
	FailedWorkers                       int    `json:"failedWorkers"`
	ActiveOperations                    int    `json:"activeOperations"`
	QueuedOperations                    int    `json:"queuedOperations"`
	MaxConcurrency                      int    `json:"maxConcurrency"`
	QueueCapacity                       int    `json:"queueCapacity"`
	ManagedOrphans                      int    `json:"managedOrphans"`
	ExternalOrphans                     int    `json:"externalOrphans"`
	LifecycleError                      bool   `json:"lifecycleError"`
}
