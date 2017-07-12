package interactors

import "github.com/bigblind/marvin/actions/domain"

// The Registry interactor gives access to the available actions.
type Registry struct {
	r domain.ProviderRegistry
}

// Group represents a group of actions.
type Group struct {
	Name     string
	Provider string
	actions  []domain.ActionMeta
}

func NewRegistryInteractor() Registry {
	return Registry{domain.Registry}
}

// GetActionGroups returns a list of available groups
func (r Registry) GetActionGroups() []Group {
	gs := make([]Group, 0)
	for _, pm := range r.r.Providers() {
		p := r.r.Provider(pm.Key)
		for _, g := range p.Groups() {
			gs = append(gs, Group{g.Name(), pm.Key, g.Actions()})
		}
	}

	return gs
}
