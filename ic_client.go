package main

import (
	"errors"
	"fmt"
	"log"

	"github.com/aviate-labs/agent-go"
	cert "github.com/aviate-labs/agent-go/certification"
	"github.com/aviate-labs/agent-go/certification/hashtree"
	"github.com/aviate-labs/agent-go/principal"
	"github.com/fxamacker/cbor/v2"
)

type CertifiedBlock struct {
	Certificate []byte `ic:"certificate" json:"certificate"`
	Data        []byte `ic:"data" json:"data"`
	Witness     []byte `ic:"witness" json:"witness"`
}

type Object struct {
	Data []byte `ic:"data" json:"data"`
}

type Result struct {
	Ok  *CertifiedBlock `ic:"Ok,variant"`
	Err *string         `ic:"Err,variant"`
}

type Result1 struct {
	Ok *struct {
	} `ic:"Ok,variant"`
	Err *string `ic:"Err,variant"`
}

// Agent is a client for the "test" canister.
type Agent struct {
	*agent.Agent
	CanisterId principal.Principal
}

// NewAgent creates a new agent for the "test" canister.
func NewAgent(canisterId principal.Principal, config agent.Config) (*Agent, error) {
	a, err := agent.New(config)
	if err != nil {
		return nil, err
	}
	return &Agent{
		Agent:      a,
		CanisterId: canisterId,
	}, nil
}

// Fetch calls the "fetch" method on the "test" canister.
func (a Agent) Fetch(arg0 string) (*CertifiedBlock, error) {
	var r0 Result
	if err := a.Agent.Query(
		a.CanisterId,
		"fetch",
		[]any{arg0},
		[]any{&r0},
	); err != nil {
		return nil, err
	}

	if r0.Err != nil {
		return nil, errors.New(*r0.Err)
	}

	return r0.Ok, nil
}

// Store calls the "store" method on the "test" canister.
func (a Agent) Store(arg0 string, arg1 Object) (*Result1, error) {
	var r0 Result1
	if err := a.Agent.Call(
		a.CanisterId,
		"store",
		[]any{arg0, arg1},
		[]any{&r0},
	); err != nil {
		return nil, err
	}
	return &r0, nil
}
func VerifyDataFromIC(certificate []byte, rootKey []byte, canister principal.Principal, witness []byte, data []byte) (cert.Certificate, error) {

	var c cert.Certificate
	if err := cbor.Unmarshal(certificate, &c); err != nil {
		return c, err
	}

	// Step 1: vetify the certification/signature. Typically you'd use cert.VerifyCertifiedData(...) function
	// but here we must drill into the hash tree and show inclusion using the witness
	if err := cert.VerifyCertificate(c, canister, rootKey); err != nil {
		log.Println(err)
		return c, err
	}

	// Step 2:	Showing inclusion in the hashtree, as only the root hash of the hashtree is certified.
	providedRootHash, err := c.Tree.Lookup(
		hashtree.Label("canister"),
		canister.Raw,
		hashtree.Label("certified_data"))
	if err != nil {
		return c, err
	}

	var rootHash [32]byte
	copy(rootHash[:], providedRootHash)

	ht, err := hashtree.Deserialize(witness)
	if err != nil {
		return c, err
	}
	witnessHash := ht.Reconstruct()

	if witnessHash != rootHash {
		return c, errors.New(fmt.Sprintf("witness hash %x doesn't match known root hash %x", witnessHash, rootHash))
	}

	fmt.Println(hashtree.AllPaths(ht))
	fmt.Println(string(data))
	return c, nil
}
