package authentication

import (
	"encoding/pem"
	"errors"
	"strings"
	"sync"

	"github.com/dgrijalva/jwt-go"
)

//go:generate counterfeiter -o fakes/fake_token.go . Token
type Token interface {
	DecodeToken(userToken string, desiredPermissions ...string) error
	CheckPublicToken() error
}

type NullToken struct{}

func (_ NullToken) DecodeToken(_ string, _ ...string) error {
	return nil
}

func (_ NullToken) CheckPublicToken() error {
	return nil
}

type accessToken struct {
	uaaPublicKey  string
	uaaKeyFetcher UaaKeyFetcher
	rwlock        sync.RWMutex
}

func NewAccessToken(uaaPublicKey string, uaaKeyFetcher UaaKeyFetcher) Token {
	return &accessToken{
		uaaPublicKey:  uaaPublicKey,
		uaaKeyFetcher: uaaKeyFetcher,
		rwlock:        sync.RWMutex{},
	}
}

func (accessToken *accessToken) DecodeToken(userToken string, desiredPermissions ...string) error {
	var err error

	jwtToken, err := checkTokenFormat(userToken)
	if err != nil {
		return err
	}

	var token *jwt.Token
	var uaaKey string
	forceUaaKeyFetch := false

	for i := 0; i < 2; i++ {
		uaaKey, err = accessToken.getUaaTokenKey(forceUaaKeyFetch)

		if err != nil {
			return err
		}

		token, err = jwt.Parse(jwtToken, func(t *jwt.Token) (interface{}, error) {
			return []byte(uaaKey), nil
		})

		if err != nil {
			if matchesError(err, jwt.ValidationErrorSignatureInvalid) {
				forceUaaKeyFetch = true
				continue
			}
			return err
		}
	}

	if err != nil {
		return err
	}

	hasPermission := false
	permissions := token.Claims["scope"]

	a := permissions.([]interface{})

	for _, permission := range a {
		for _, desiredPermission := range desiredPermissions {
			if permission.(string) == desiredPermission {
				hasPermission = true
				break
			}
		}
	}

	if !hasPermission {
		err = errors.New("Token does not have '" + strings.Join(desiredPermissions, "', '") + "' scope")
		return err
	}

	return nil
}

func (accessToken *accessToken) getUaaPublicKey() string {
	accessToken.rwlock.RLock()
	defer accessToken.rwlock.RUnlock()
	return accessToken.uaaPublicKey
}

func (accessToken *accessToken) CheckPublicToken() error {
	return checkPublicKey(accessToken.getUaaPublicKey())
}

func checkPublicKey(key string) error {
	var block *pem.Block
	if block, _ = pem.Decode([]byte(key)); block == nil {
		return errors.New("Public uaa token must be PEM encoded")
	}
	return nil
}

func (accessToken *accessToken) getUaaTokenKey(forceFetch bool) (string, error) {
	if accessToken.getUaaPublicKey() == "" || forceFetch {
		key, err := accessToken.uaaKeyFetcher.FetchKey()
		if err != nil {
			return key, err
		}
		err = checkPublicKey(key)
		if err != nil {
			return "", err
		}
		accessToken.rwlock.Lock()
		defer accessToken.rwlock.Unlock()
		accessToken.uaaPublicKey = key
		return accessToken.uaaPublicKey, nil
	}

	return accessToken.getUaaPublicKey(), nil
}

func checkTokenFormat(token string) (string, error) {
	tokenParts := strings.Split(token, " ")
	if len(tokenParts) != 2 {
		return "", errors.New("Invalid token format")
	}

	tokenType, userToken := tokenParts[0], tokenParts[1]
	if tokenType != "bearer" {
		return "", errors.New("Invalid token type: " + tokenType)
	}

	return userToken, nil
}

func matchesError(err error, errorType uint32) bool {
	if validationError, ok := err.(*jwt.ValidationError); ok {
		return validationError.Errors&errorType == errorType
	}
	return false
}
