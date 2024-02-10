//go:build edgex
// +build edgex

package edgex_vault

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"
)

type VaultSecret struct {
	scheme          string
	host            string
	port            int16
	secretName      string
	vaultToken      string
	renewalFactor   float64
	client          *http.Client
	authContext     map[string]interface{}
	logger          *slog.Logger
	renewalCallback func()
	started         bool
	wg              sync.WaitGroup
}

var vaultSecrets = make(map[string]*VaultSecret, 10)

func NewVaultSecret(vaultAddr, secretName, initialSecret string) *VaultSecret {
	secretBytes, err := os.ReadFile(initialSecret)
	if err != nil {
		panic(fmt.Errorf("could not read initial secret at :%s", initialSecret))
	}

	v := &VaultSecret{
		client:        &http.Client{},
		scheme:        "http",
		host:          vaultAddr,
		port:          8200,
		secretName:    secretName,
		renewalFactor: 0.75,
		logger:        slog.Default(),
		wg:            sync.WaitGroup{},
	}

	v.renewalCallback = v.autoRenew

	if initSecErr := v.readToken(secretBytes); initSecErr != nil {
		panic(fmt.Errorf("exchangeVaultToken failed. error reading initial secret: %v", initSecErr))
	}

	found := vaultSecrets[v.vaultToken]
	if found != nil {
		return found
	}

	vaultSecrets[v.vaultToken] = v
	return v
}

func (v *VaultSecret) Start() {
	if v.started {
		v.wg.Wait()
		return
	}
	v.started = true
	v.wg.Add(1)
	if exchangeErr := v.exchangeVaultToken(); exchangeErr != nil {
		panic(fmt.Errorf("exchangeVaultToken failed. error exchanging token: %v", exchangeErr))
	}
	v.wg.Done()
}

func (v *VaultSecret) autoRenew() {
	renewErr := v.renewToken()
	if renewErr != nil {
		panic(renewErr)
	}
}

func (v *VaultSecret) renewToken() error {
	url := fmt.Sprintf("%s://%s:%d/v1/auth/token/renew-self", v.scheme, v.host, v.port)

	req, newReqErr := http.NewRequest("POST", url, nil)
	if newReqErr != nil {
		return fmt.Errorf("renewToken failed. error creating request: %v", newReqErr)
	}

	respBody, callErr := v.callVault(req)
	if callErr != nil {
		return callErr
	}
	defer func() { _ = respBody.Close() }()

	body, readErr := io.ReadAll(respBody)
	if readErr != nil {
		return fmt.Errorf("renewToken failed. error reading response body: %v", readErr)
	}

	if readTokenErr := v.readToken(body); readTokenErr != nil {
		return fmt.Errorf("the vault token could not be refreshed! %v", readTokenErr)
	}
	err := v.exchangeVaultToken()
	if err != nil {
		return err
	}
	return nil
}

func (v *VaultSecret) exchangeVaultToken() error {
	url := fmt.Sprintf("%s://%s:%d/v1/identity/oidc/token/"+v.secretName, v.scheme, v.host, v.port)

	req, newReqErr := http.NewRequest("GET", url, nil)
	if newReqErr != nil {
		return fmt.Errorf("exchangeVaultToken failed. error creating request: %v", newReqErr)
	}

	respBody, callErr := v.callVault(req)
	if callErr != nil {
		return callErr
	}
	defer func() { _ = respBody.Close() }()

	body, readErr := io.ReadAll(respBody)
	if readErr != nil {
		return fmt.Errorf("exchangeVaultToken failed. error reading response body: %v", readErr)
	}

	var resp map[string]interface{}
	if unmarshalErr := json.Unmarshal(body, &resp); unmarshalErr != nil {
		return fmt.Errorf("exchangeVaultToken failed. error decoding JSON: %v", unmarshalErr)
	}
	v.authContext = resp["data"].(map[string]interface{})

	// using the result, setup a callback schedule to keep the token fresh in case it's ever needed
	ttl := v.authContext["ttl"]
	callbackTime := time.Duration(ttl.(float64) * v.renewalFactor * float64(time.Second))
	slog.Info(fmt.Sprintf("vault token valid for %f seconds. token renewal will occur in %v", ttl, callbackTime))

	time.AfterFunc(callbackTime, v.renewalCallback)
	return nil
}

func (v *VaultSecret) readToken(content []byte) error {
	var vaultResponse struct {
		Auth struct {
			ClientToken string `json:"client_token"`
		} `json:"auth"`
	}

	err := json.Unmarshal(content, &vaultResponse)
	if err != nil {
		return fmt.Errorf("exchangeVaultToken failed. could not read vault token: %v", err)
	}

	v.vaultToken = vaultResponse.Auth.ClientToken
	return nil
}

func (v *VaultSecret) callVault(req *http.Request) (io.ReadCloser, error) {
	req.Header.Set("X-Vault-Token", v.vaultToken)
	resp, err := v.client.Do(req)
	if err != nil {
		return resp.Body, fmt.Errorf("error making request: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		return resp.Body, fmt.Errorf("error making request, http status code not OK: %d", resp.StatusCode)
	}
	return resp.Body, nil
}

func (v *VaultSecret) Jwt() string {
	return v.authContext["token"].(string)
}
