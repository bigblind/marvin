package actions

import (
	"context"
	"encoding/gob"
	"fmt"
	"github.com/marvin-automator/marvin/actions"
	"golang.org/x/oauth2"
	"reflect"
	"strings"
)

type action struct {
	info    actions.Info
	runFunc reflect.Value
}

func (a *action) Info() actions.Info {
	return a.info
}

func (a *action) Run(input interface{}, ctx context.Context) (interface{}, error) {
	v := reflect.ValueOf(input)
	if v.Kind() == reflect.Ptr {
		input = v.Elem().Interface()
	}
	fmt.Printf("Running %v with input %v", a.info.Name, reflect.TypeOf(input))
	retvals := a.runFunc.Call([]reflect.Value{reflect.ValueOf(input), reflect.ValueOf(ctx)})
	res := retvals[0].Interface()
	fmt.Printf("Returned value %v", res)
	if err, ok := retvals[1].Interface().(error); !ok {
		return res, nil
	} else {
		return res, err
	}
}

func (a *action) validate() {
	name := a.Info().Name

	if a.runFunc.Kind() != reflect.Func {
		panic(fmt.Sprintf("Action %v did not receive a function as runFunc", name))
	}

	ft := a.runFunc.Type()
	ctx := context.Background()
	if !(ft.NumIn() == 2 &&
		reflect.TypeOf(ctx).AssignableTo(ft.In(1))) {
		panic(fmt.Sprintf("Action %v should have a function that takes 2 arguments. The first can be any type, as long as it is json-unmarshalable, the second is a context.Context", name))
	}

	if a.info.IsTrigger {
		a.validateTrigger(ft)
		a.info.OutputType = ft.Out(0).Elem()
	} else {
		a.validateAction(ft)
		a.info.OutputType = ft.Out(0)
	}

	a.info.InputType = ft.In(0)
}

func (a *action) validateAction(ft reflect.Type) {
	name := a.Info().Name

	var e *error
	if !(ft.NumOut() == 2 && ft.Out(1).Implements(reflect.TypeOf(e).Elem())) {
		panic(fmt.Sprintf("Action %v should have a function that returns 2 values, The second must implement error.", name))
	}
}

func (a *action) validateTrigger(ft reflect.Type) {
	name := a.Info().Name

	var e *error
	if !(ft.NumOut() == 2 && ft.Out(0).Kind() == reflect.Chan && ft.Out(1).Implements(reflect.TypeOf(e).Elem()) &&
		ft.Out(0).ChanDir()&reflect.RecvDir == reflect.RecvDir) {
		panic(fmt.Sprintf("Trigger %v should have a function that returns 2 values:\n- A readable channel containing structs representing events\n - an error in case anything went wrong.", name))
	}

	if !strings.HasPrefix(a.info.Name, "on") {
		panic(fmt.Sprintf("Trigger names must start with \"on\", change the name of \"%v\"", a.info.Name))
	}
}

type Group struct {
	actions.BaseInfo
	actions map[string]actions.Action
}

func (g *Group) addAction(name, description string, svgIcon []byte, runFunc interface{}, trigger bool) {
	p := g.Info()
	info := actions.Info{
		BaseInfo:  actions.BaseInfo{name, description, svgIcon, &p},
		IsTrigger: trigger,
	}

	a := &action{
		info:    info,
		runFunc: reflect.ValueOf(runFunc),
	}

	a.validate()
	if trigger {
		gob.Register(reflect.New(a.Info().InputType).Elem().Interface())
	}

	g.actions[name] = a
}

func (g *Group) AddAction(name, description string, svgIcon []byte, runFunc interface{}) {
	g.addAction(name, description, svgIcon, runFunc, false)
}

func (g *Group) AddManualTrigger(name, description string, svgIcon []byte, runFunc interface{}) {
	g.addAction(name, description, svgIcon, runFunc, true)
}

func (g *Group) Actions() []actions.Action {
	res := make([]actions.Action, 0, len(g.actions))
	for _, a := range g.actions {
		res = append(res, a)
	}

	return res
}

type Provider struct {
	actions.BaseInfo
	groups map[string]*Group

	Requirements map[string]actions.Requirement

	OAuth2Endpoint oauth2.Endpoint
}

func (p *Provider) AddGroup(name, description string, svgIcon []byte) actions.Group {
	parent := p.Info()
	g := &Group{actions.BaseInfo{name, description, svgIcon, &parent}, make(map[string]actions.Action)}
	p.groups[name] = g
	return g
}

func (p *Provider) Groups() []actions.Group {
	res := make([]actions.Group, 0, len(p.groups))
	for _, g := range p.groups {
		res = append(res, g)
	}

	return res
}

func (p *Provider) AddRequirement(req actions.Requirement) {
	p.Requirements[req.Name()] = req
	req.Init(p.Name)
}

func (p *Provider) loadRequirementConfig() error {
	for _, req := range p.Requirements {
		err := loadRequirementConfig(p.Name, req)
		if err != nil {
			return err
		}
	}
	return nil
}

type ProviderRegistry struct {
	providers map[string]*Provider
}

func NewRegistry() *ProviderRegistry {
	return &ProviderRegistry{make(map[string]*Provider)}
}

func (r *ProviderRegistry) AddProvider(name, description string, svgIcon []byte) actions.Provider {
	p := &Provider{
		BaseInfo: actions.BaseInfo{name, description, svgIcon, nil},
		groups:   make(map[string]*Group),
	}

	r.providers[p.Name] = p

	return p
}

func (r *ProviderRegistry) Providers() []actions.Provider {
	ps := make([]actions.Provider, 0, len(r.providers))
	for _, p := range r.providers {
		ps = append(ps, p)
	}

	return ps
}

func (r *ProviderRegistry) Provider(name string) *Provider {
	p, _ := r.providers[name]
	return p
}

func (r *ProviderRegistry) GetAction(provider, group, action string) (actions.Action, error) {
	if p, ok := r.providers[provider]; ok {
		if g, ok := p.groups[group]; ok {
			if a, ok := g.actions[action]; ok {
				return a, nil
			}
			return nil, fmt.Errorf("Group %v->%v has no action %v", provider, group, action)
		}
		return nil, fmt.Errorf("Provider %v has no group %v", provider, group)
	}
	return nil, fmt.Errorf("no provider: %v", provider)
}

func (p *ProviderRegistry) LoadProviderConfigs() error {
	for _, p := range p.Providers() {
		err := p.loadRequirementConfig()
		if err != nil {
			return err
		}
	}

	return nil
}

var GlobalRegistry *ProviderRegistry

func init() {
	GlobalRegistry = NewRegistry()
	actions.Registry = GlobalRegistry
}
