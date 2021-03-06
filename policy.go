package main

import (
	"fmt"
	"github.com/franela/goreq"
	"log"
	"path"
)

type policyLoadError struct {
	Err error `json:"error"`
}

func (ple policyLoadError) Error() string {
	return fmt.Sprintf("Error loading policy from vault: %v", ple.Err)
}

type policy struct {
	Policies []string          `json:"policies"`
	Meta     map[string]string `json:"meta,omitempty"`
	Ttl      int               `json:"ttl,omitempty"`
	NumUses  int               `json:"num_users,omitempty"`
}

type policies map[string]*policy

var defaultPolicy = &policy{
	Ttl: 21600,
}
var defaultPolicies = map[string]*policy{
	"*": &policy{
		Policies: []string{"default"},
		Ttl:      21600,
	},
}
var activePolicies = make(policies)

func (p policies) Get(key string) *policy {
	if pol, ok := p[key]; ok {
		return pol
	} else if pol, ok := p["*"]; ok {
		return pol
	} else {
		return defaultPolicy
	}
}

func (p policies) Load(authToken string) error {
	r, err := VaultRequest{goreq.Request{
		Uri:             vaultPath(path.Join("/v1/secret", config.Vault.GkPolicies), ""),
		MaxRedirects:    10,
		RedirectHeaders: true,
	}.WithHeader("X-Vault-Token", authToken)}.Do()
	if err == nil {
		defer r.Body.Close()
		switch r.StatusCode {
		case 200:
			resp := struct {
				Data policies `json:"data"`
			}{}
			if err := r.Body.FromJsonTo(&resp); err == nil {
				for k, _ := range p {
					delete(p, k)
				}
				for k, v := range resp.Data {
					p[k] = v
				}
				return nil
			} else {
				return policyLoadError{fmt.Errorf("There was an error decoding policy from vault. This can occur when using vault-cli to save the policy json, as vault-cli saves it as a string rather than a json object.")}
			}
		case 404:
			log.Printf("There was no policy in the secret backend at %v. Tokens created will have the default vault policy.", config.Vault.GkPolicies)
			for k, _ := range p {
				delete(p, k)
			}
			for k, v := range defaultPolicies {
				p[k] = v
			}
			return nil
		default:
			var e vaultError
			e.Code = r.StatusCode
			if err := r.Body.FromJsonTo(&e); err == nil {
				return policyLoadError{e}
			} else {
				e.Errors = []string{"communication error."}
				return policyLoadError{e}
			}
		}
	} else {
		return policyLoadError{err}
	}
}
