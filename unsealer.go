package main

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/franela/goreq"
	"hash"
	"io/ioutil"
	"net"
	"strings"
)

type vaultError struct {
	Code   int      `json:"-"`
	Errors []string `json:"errors"`
}

func (e vaultError) Error() string {
	return fmt.Sprintf("%d: %s", e.Code, strings.Join(e.Errors, ", "))
}

type vaultTokenResp struct {
	Auth struct {
		ClientToken   string `json:"client_token"`
		LeaseDuration int    `json:"lease_duration"`
		TTL           int    `json:"ttl"`
	} `json:"auth"`
}

type Unsealer interface {
	Token() (string, error)
	Name() string
}

type TokenUnsealer struct {
	AuthToken string
}

func (t TokenUnsealer) Token() (string, error) {
	r, err := goreq.Request{
		Uri: vaultPath("/v1/auth/token/lookup-self", ""),
	}.WithHeader("X-Vault-Token", t.AuthToken).Do()
	if err == nil {
		defer r.Body.Close()
		switch r.StatusCode {
		case 200:
			return t.AuthToken, nil
		default:
			var e vaultError
			e.Code = r.StatusCode
			if err := r.Body.FromJsonTo(&e); err == nil {
				return "", e
			} else {
				e.Errors = []string{"communication error."}
				return "", e
			}
		}
	} else {
		return "", err
	}
}

func (t TokenUnsealer) Name() string {
	return "token"
}

type genericUnsealer struct{}

func (g genericUnsealer) Token(req goreq.Request) (string, error) {
	r, err := req.Do()
	if err == nil {
		defer r.Body.Close()
		switch r.StatusCode {
		case 200:
			var t vaultTokenResp
			if err := r.Body.FromJsonTo(&t); err == nil {
				return t.Auth.ClientToken, nil
			} else {
				return "", err
			}
		default:
			var e vaultError
			e.Code = r.StatusCode
			if err := r.Body.FromJsonTo(&e); err == nil {
				return "", e
			} else {
				e.Errors = []string{"communication error."}
				return "", e
			}
		}
	} else {
		return "", err
	}
}

type AppIdUnsealer struct {
	AppId           string
	UserIdMethod    string
	UserIdInterface string
	UserIdPath      string
	UserIdHash      string
	UserIdSalt      string
	genericUnsealer
}

var errUnknownUserIdMethod = errors.New("Unknown method specified for user id.")
var errUnknownHashMethod = errors.New("Unknown hash method specified for user id.")

func (a AppIdUnsealer) Token() (string, error) {
	body := struct {
		UserId string `json:"user_id"`
	}{}
	switch a.UserIdMethod {
	case "mac":
		if iface, err := net.InterfaceByName(a.UserIdInterface); err == nil {
			body.UserId = iface.HardwareAddr.String()
		} else {
			return "", err
		}
	case "file":
		if b, err := ioutil.ReadFile(a.UserIdPath); err == nil {
			body.UserId = string(b)
		} else {
			return "", err
		}
	default:
		return "", errUnknownUserIdMethod
	}
	var hasher hash.Hash
	switch a.UserIdHash {
	case "md5":
		hasher = md5.New()
	case "sha1":
		hasher = sha1.New()
	case "sha256":
		hasher = sha256.New()
	case "":

	default:
		return "", errUnknownHashMethod
	}
	if hasher != nil {
		h := body.UserId
		if a.UserIdSalt != "" {
			h = a.UserIdSalt + "$" + h
		}
		if _, err := hasher.Write([]byte(h)); err == nil {
			body.UserId = hex.EncodeToString(hasher.Sum(nil))
		} else {
			return "", err
		}
	}
	return a.genericUnsealer.Token(goreq.Request{
		Uri:    vaultPath("/v1/auth/app-id/login/"+a.AppId, ""),
		Method: "POST",
		Body:   body,
	})
}

func (a AppIdUnsealer) Name() string {
	return "app-id"
}

type GithubUnsealer struct {
	PersonalToken string
	genericUnsealer
}

func (gh GithubUnsealer) Token() (string, error) {
	return gh.genericUnsealer.Token(goreq.Request{
		Uri:    vaultPath("/v1/auth/github/login", ""),
		Method: "POST",
		Body: struct {
			Token string `json:"token"`
		}{gh.PersonalToken},
	})
}

func (gh GithubUnsealer) Name() string {
	return "github"
}

type UserpassUnsealer struct {
	Username string
	Password string
	genericUnsealer
}

func (u UserpassUnsealer) Token() (string, error) {
	return u.genericUnsealer.Token(goreq.Request{
		Uri:    vaultPath("/v1/auth/userpass/login/"+u.Username, ""),
		Method: "POST",
		Body: struct {
			Password string `json:"password"`
		}{u.Password},
	})
}

func (u UserpassUnsealer) Name() string {
	return "userpass"
}