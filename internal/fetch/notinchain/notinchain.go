// Package notinchain provides a temporary sollution for missing data in the blockchain.
// Remove the THOR node REST URL from the configuration once removed.
package notinchain

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"gitlab.com/thorchain/midgard/internal/util/midlog"
)

// BaseURL defines the REST root.
var BaseURL string

var Client http.Client

// TODO(kashif) we can merge this in future into a better caching layer
// not sure at this point if its necessary or not
var (
	cacheDuration   time.Duration  = 5 * time.Second
	nodesCache      []*NodeAccount // For nodeaccounts
	nodesCachedAt   time.Time
	reserveAddress  string
	reserveCachedAt time.Time
)

type NodeCache struct {
	Node     *NodeAccount
	CachedAt time.Time
}

var nodeCache map[string]*NodeCache = make(map[string]*NodeCache) // For nodeaccount

// Return a cached version of nodeaccounts to reduce load on thorchain nodes
func CachedNodeAccountsLookup() ([]*NodeAccount, error) {
	if nodesCache != nil && time.Now().Before(nodesCachedAt.Add(cacheDuration)) {
		return nodesCache, nil
	}

	newNodes, err := NodeAccountsLookup()
	if err != nil {
		return nil, err
	}

	nodesCache = newNodes
	nodesCachedAt = time.Now()
	return newNodes, err
}

func CachedReserveLookup() (string, error) {
	if reserveAddress != "" && time.Now().Before(reserveCachedAt.Add(cacheDuration*50)) {
		return reserveAddress, nil
	}

	reserveAddress, err := ReserveLookup()
	if err != nil {
		return "", err
	}

	reserveCachedAt = time.Now()
	return reserveAddress, err
}

// Return a cached version of nodeaccount to reduce load on thorchain nodes
func CachedNodeAccountLookup(address string) (*NodeAccount, error) {
	c := nodeCache[address]
	if c != nil && time.Now().Before(c.CachedAt.Add(cacheDuration)) {
		return c.Node, nil
	}

	newNode, err := NodeAccountLookup(address)
	if err != nil {
		return nil, err
	}

	nodeCache[address] = &NodeCache{
		Node:     newNode,
		CachedAt: time.Now(),
	}
	return newNode, err
}

type JailInfo struct {
	NodeAddr      string `json:"node_address"`
	ReleaseHeight int64  `json:"release_height"`
	Reason        string `json:"reason"`
}

type PublicKeys struct {
	Secp256k1 string `json:"secp256k1"`
	Ed25519   string `json:"ed25519"`
}

type Provider struct {
	Address string `json:"bond_address"`
	Bond    string `json:"bond"`
}

type BondProvider struct {
	Providers       []Provider `json:"providers"`
	NodeOperatorFee string     `json:"node_operator_fee"`
}

type NodeAccount struct {
	NodeAddr         string       `json:"node_address"`
	Status           string       `json:"status"`
	TotalBond        int64        `json:"total_bond,string"`
	PublicKeys       PublicKeys   `json:"pub_key_set"`
	RequestedToLeave bool         `json:"requested_to_leave"`
	ForcedToLeave    bool         `json:"forced_to_leave"`
	LeaveHeight      int64        `json:"leave_height"`
	IpAddress        string       `json:"ip_address"`
	Version          string       `json:"version"`
	SlashPoints      int64        `json:"slash_points"`
	Jail             JailInfo     `json:"jail"`
	CurrentAward     int64        `json:"current_award,string"`
	BondProviders    BondProvider `json:"bond_providers"`
}

// Get all nodes from the thorchain api
func NodeAccountsLookup() ([]*NodeAccount, error) {
	resp, err := Client.Get(BaseURL + "/nodes")
	if err != nil {
		return nil, fmt.Errorf("node accounts unavailable from REST on %w", err)
	}
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("node accounts REST HTTP status %q, want 2xx", resp.Status)
	}
	var accounts []*NodeAccount
	if err := json.NewDecoder(resp.Body).Decode(&accounts); err != nil {
		return nil, fmt.Errorf("node accounts irresolvable from REST on %w", err)
	}
	return accounts, nil
}

type ReserveModule struct {
	Name    string `json:"name"`
	Address string `json:"address"`
	Coins   []Coin `json:"coins"`
}

func ReserveLookup() (string, error) {
	resp, err := Client.Get(BaseURL + "/balance/module/reserve")
	if err != nil {
		return "", fmt.Errorf("reserve balance unavailable from REST on %w", err)
	}
	if resp.StatusCode/100 != 2 {
		return "", fmt.Errorf("reserve balance REST HTTP status %q, want 2xx", resp.Status)
	}
	var moduleAddress ReserveModule
	if err := json.NewDecoder(resp.Body).Decode(&moduleAddress); err != nil {
		return "", fmt.Errorf("reserve balance irresolvable from REST on %w", err)
	}
	return moduleAddress.Address, nil
}

func AsyncNodeAccountsLookup(wg *sync.WaitGroup, ch chan<- interface{}) {
	defer wg.Done()
	ret, err := CachedNodeAccountsLookup()
	if err != nil {
		midlog.Err(err)
	}
	ch <- ret
}

// Get node details by address from the thorchain api
func NodeAccountLookup(addr string) (*NodeAccount, error) {
	resp, err := Client.Get(BaseURL + "/node/" + addr)
	if err != nil {
		return nil, fmt.Errorf("node account unavailable from REST on %w", err)
	}
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("node account REST HTTP status %q, want 2xx", resp.Status)
	}
	var account *NodeAccount
	if err := json.NewDecoder(resp.Body).Decode(&account); err != nil {
		return nil, fmt.Errorf("node account irresolvable from REST on %w", err)
	}
	return account, nil
}

type Network struct {
	TotalReserve          int64 `json:"total_reserve,string"`
	EffectiveSecurityBond int64 `json:"effective_security_bond,string"`
}

// Get vault data from the thorchain api
func NetworkLookup() (*Network, error) {
	resp, err := Client.Get(BaseURL + "/network")
	if err != nil {
		return nil, fmt.Errorf("network data unavailable from REST on %w", err)
	}
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("vault data REST HTTP status %q, want 2xx", resp.Status)
	}
	var data *Network
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("vault data irresolvable from REST on %w", err)
	}
	return data, nil
}

func AsyncNetworkLookup(wg *sync.WaitGroup, ch chan<- interface{}) {
	defer wg.Done()
	ret, err := NetworkLookup()
	if err != nil {
		midlog.Err(err)
	}
	ch <- ret
}

type Coin struct {
	Asset    string `json:"asset"`
	Amount   string `json:"amount"`
	Decimals *int64 `json:"decimals,omitempty"`
}
type AsgardVaults struct {
	Coins  []Coin `json:"coins"`
	Status string `json:"status"`
}

// Get asgard vault data from the thorchain api
func AsgardVaultsLookup() (*[]AsgardVaults, error) {
	resp, err := Client.Get(BaseURL + "/vaults/asgard")
	if err != nil {
		return nil, fmt.Errorf("asgard vault data unavailable from REST on %w", err)
	}
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("asgard vault data REST HTTP status %q, want 2xx", resp.Status)
	}
	var data *[]AsgardVaults
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("asgard vault data irresolvable from REST on %w", err)
	}
	return data, nil
}

func AsyncAsgardVaultsLookup(wg *sync.WaitGroup, ch chan<- interface{}) {
	defer wg.Done()
	ret, err := AsgardVaultsLookup()
	if err != nil {
		midlog.Err(err)
	}
	ch <- ret
}

type Constants struct {
	Int64Values map[string]int64 `json:"int_64_values"`
}

var constants *Constants

func LoadConstants() error {
	resp, err := Client.Get(BaseURL + "/constants")
	if err != nil {
		return fmt.Errorf("constants unavailable from REST on %w", err)
	}
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("constants REST HTTP status %q, want 2xx", resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(&constants); err != nil {
		return fmt.Errorf("constants irresolvable from REST on %w", err)
	}
	return nil
}

// Looks up thorchain constants, query is run once then cached in memory
func GetConstants() *Constants {
	return constants
}
