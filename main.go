package main

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/CommoDor64/icdaserver/icutils"

	"github.com/aviate-labs/agent-go"
	"github.com/aviate-labs/agent-go/principal"
	"github.com/offchainlabs/nitro/das/dastree"
)

const (
	StateCreated = iota
	StatePopulated
	StateCommited
	StateFailed
)

type Batch struct {
	State   uint
	Timeout uint
	Data    []byte
}

type Batches struct {
	mu      sync.Mutex
	batches []Batch
}

func (b *Batches) GetSlotById(id int) ([]byte, int, uint, int) {

	data := make([]byte, len(b.batches[id].Data))
	copy(data, b.batches[id].Data)

	return data, int(b.batches[id].State), b.batches[id].Timeout, 0
}

func (b *Batches) CreateSlot(timeout uint) int {
	b.mu.Lock()
	defer b.mu.Unlock()

	slotId := len(b.batches)
	b.batches = append(b.batches, Batch{
		State:   StateCreated,
		Data:    nil,
		Timeout: timeout,
	})
	return slotId
}

func (b *Batches) InsertToSlot(id int, data []byte) int {
	b.mu.Lock()
	defer b.mu.Unlock()

	if id >= len(b.batches) {
		return -1

	}

	if b.batches[id].State != StateCreated {
		return -2
	}

	b.batches[id].Data = data
	b.batches[id].State = StatePopulated

	return 0
}

func (b *Batches) FinalizeSlot(id int) int {
	b.mu.Lock()
	defer b.mu.Unlock()

	if id >= len(b.batches) {
		return -1
	}

	if b.batches[id].State != StatePopulated {
		return -2
	}

	b.batches[id].State = StateCommited
	return 0
}

type Batcher struct {
	batches Batches
}

func (b *Batcher) GetBatchById(batchId uint64) (*Batch, error) {
	data, state, timeout, err := b.batches.GetSlotById(int(batchId))
	if err != 0 {
		return nil, errors.New("couldn't get batch by id")
	}
	return &Batch{
		State:   uint(state),
		Data:    data,
		Timeout: timeout,
	}, nil
}

func (b *Batcher) CreateBatch(timeout uint) (uint64, error) {
	slotId := b.batches.CreateSlot(timeout)

	if slotId < 0 {
		return 0, errors.New("could not apppend batch")
	}

	return uint64(slotId), nil
}

func (b *Batcher) StoreBatch(batchId uint64, data []byte) error {
	res := b.batches.InsertToSlot(int(batchId), data)
	if res != 0 {
		return errors.New("could not store data in batch")
	}

	return nil
}

func (b *Batcher) CommitBatch(batchId uint64) error {
	err := b.batches.FinalizeSlot(int(batchId))
	if err != 0 {
		return errors.New("could not commit batch")
	}

	return nil
}

var BCOUNTER = Batcher{
	batches: Batches{
		mu:      sync.Mutex{},
		batches: []Batch{},
	},
}

type DASService struct {
	Agent      *icutils.Agent
	Canister   principal.Principal
	keysetHash string
	Batcher
}

type StartChunkedStoreArgs struct {
	Timestamp      uint64 `json:"timestamp,omitempty"`
	NumberOfChunks uint64 `json:"nChunks,omitempty"`
	ChunkSize      uint64 `json:"chunkSize,omitempty"`
	TotalSize      uint64 `json:"totalSize,omitempty"`
	Timeout        uint64 `json:"timeout,omitempty"`
	Signature      []byte `json:"sig,omitempty"`
}

type StartChunkedStoreArgs2 []string

type StartChunkedStoreReply struct {
	BatchId string `json:"batchId,omitempty"` //uint64
}

func (d *DASService) StartChunkedStore(args *StartChunkedStoreArgs) (*StartChunkedStoreReply, error) {
	log.Println("das_startChunkedStore called")

	batchId, err := BCOUNTER.CreateBatch(uint(args.Timeout))

	if err != nil {
		return nil, err
	}
	s := "0x" + strconv.FormatUint(batchId, 16)

	return &StartChunkedStoreReply{BatchId: s}, nil
}

type SendChunkArgs struct {
	BatchId   uint64 `json:"batchId,omitempty"`
	ChunkId   uint64 `json:"chunkId,omitempty"`
	Message   []byte `json:"message,omitempty"`
	Signature []byte `json:"sig,omitempty"`
}
type SendChunkReply struct {
}

func (d *DASService) SendChunk(args *SendChunkArgs) (*SendChunkReply, error) {
	log.Println("das_sendChunk called", "with batchId", args.BatchId, "and chunk", args.ChunkId)
	if err := BCOUNTER.StoreBatch(args.BatchId, args.Message); err != nil {
		return nil, err
	}
	return nil, nil
}

type CommitChunkedStoreArgs struct {
	BatchId   uint64 `json:"batchId,omitempty"`
	Signature []byte `json:"sig,omitempty"`
}
type CommitChunkedStoreReply struct {
	Canister    string `json:"canister,omitempty"`    //real type string
	DataHash    string `json:"dataHash,omitempty"`    //real type []byte
	Timeout     string `json:"timeout,omitempty"`     //real type uint64
	SignersMask string `json:"signersMask,omitempty"` //real type  uint64
	KeysetHash  string `json:"keysetHash,omitempty"`  //real type []byte
	Signature   string `json:"sig,omitempty"`         //real type []byte
	Version     string `json:"version,omitempty"`     //real type uint64
}

func (d *DASService) CommitChunkedStore(args *CommitChunkedStoreArgs) (*CommitChunkedStoreReply, error) {
	log.Println("das_commitChunkedStore called")

	batch, err := BCOUNTER.GetBatchById(args.BatchId)
	if err != nil {
		panic(err)
	}

	h := dastree.Hash(batch.Data).Hex()
	_, err = d.Agent.Store(
		h,
		batch.Data)

	if err != nil {
		panic(err)
	}

	blockHash := h

	cb, err := d.Agent.Fetch(blockHash)
	if err != nil {
		panic(err)
	}

	_, err = icutils.VerifyDataFromIC(cb.Certificate, d.Agent.GetRootKey(), d.Canister, cb.Witness, dastree.Hash(batch.Data).Bytes())
	if err != nil {
		return nil, err
	}

	_, err = json.Marshal(cb)
	if err != nil {
		panic(err)
	}

	err = BCOUNTER.CommitBatch(args.BatchId)
	if err != nil {
		panic(err)
	}

	b, err := json.Marshal(cb)
	if err != nil {
		panic(err)
	}

	return &CommitChunkedStoreReply{
		Canister:    d.Canister.String(),
		DataHash:    h,
		Timeout:     "0x" + strconv.FormatUint(uint64(batch.Timeout), 16),
		SignersMask: "0x1",
		KeysetHash:  d.keysetHash,
		Signature:   "0x" + hex.EncodeToString(b),
		Version:     "0x1",
	}, nil
}

type GetByHashResponse struct {
	Canister         string `json:"canister,omitempty"`
	Certificate      []byte `json:"certificate,omitempty"`
	Witness          []byte `json:"witness,omitempty"`
	Data             string `json:"data,omitempty"`
	ExpirationPolicy string `json:"expirationPolicy,omitempty"`
}

func RESTHandler(service *DASService) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {

		hash := strings.Split(r.URL.Path, "/")[2]

		if hash == "" || !strings.HasPrefix(hash, "0x") {
			log.Println("hash is malformed: expected 0x prefixed hash, got:", hash)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		cb, err := service.Agent.Fetch(hash)
		if err != nil {
			panic(err)
		}

		_, err = icutils.VerifyDataFromIC(cb.Certificate, service.Agent.GetRootKey(), service.Canister, cb.Witness, dastree.Hash(cb.Data).Bytes())
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		encodedData := make([]byte, base64.StdEncoding.EncodedLen(len(cb.Data)))
		base64.StdEncoding.Encode(encodedData, cb.Data)

		res := GetByHashResponse{
			Canister:         service.Canister.String(),
			Certificate:      cb.Certificate,
			Witness:          cb.Witness,
			Data:             string(encodedData),
			ExpirationPolicy: "",
		}

		b, err := json.Marshal(res)
		if err != nil {
			return
		}

		w.Write(b)
	}
}

func JSONRPCHandler(service *DASService) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
			ID     interface{}     `json:"id"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		var (
			err error
		)

		if err != nil {
			log.Println(err)
			return
		}

		switch req.Method {
		case "das_startChunkedStore":
			var args2 StartChunkedStoreArgs2
			if err := json.Unmarshal(req.Params, &args2); err != nil {
				http.Error(w, "Invalid das_startChunkedStore params", http.StatusBadRequest)
				return
			}

			ts, err := strconv.ParseUint(args2[0][2:], 16, 64)
			if err != nil {
				log.Println(err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			numberOfChunks, err := strconv.ParseUint(args2[1][2:], 16, 64)
			if err != nil {
				log.Println(err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			chunkSize, err := strconv.ParseUint(args2[2][2:], 16, 64)
			if err != nil {
				log.Println(err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			totalSize, err := strconv.ParseUint(args2[3][2:], 16, 64)
			if err != nil {
				log.Println(err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			timeout, err := strconv.ParseUint(args2[4][2:], 16, 64)
			if err != nil {
				log.Println(err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			signature, err := hex.DecodeString(args2[5][2:])
			if err != nil {
				log.Println(err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			reply, err := service.StartChunkedStore(&StartChunkedStoreArgs{
				Timestamp:      ts,
				NumberOfChunks: numberOfChunks,
				ChunkSize:      chunkSize,
				TotalSize:      totalSize,
				Timeout:        timeout,
				Signature:      signature,
			})

			if err != nil {
				log.Println(err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			resp := map[string]interface{}{
				"jsonrpc": "2.0",
				"result":  reply,
				"id":      req.ID,
			}

			w.WriteHeader(200)
			if err := json.NewEncoder(w).Encode(resp); err != nil {
				log.Println(err)
			}

		case "das_sendChunk":
			var args2 []string
			if err := json.Unmarshal(req.Params, &args2); err != nil {
				log.Println(err)
				http.Error(w, "Invalid das_sendChunk params", http.StatusBadRequest)
				return
			}

			var args SendChunkArgs
			args.Message, err = hex.DecodeString(args2[len(args2)-2][2:])
			if err != nil {
				log.Println("can't decode message", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			args.BatchId, err = strconv.ParseUint(args2[0][2:], 16, 64)
			if err != nil {
				log.Println("can't decode batch number", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			reply, err := service.SendChunk(&args)
			if err != nil {
				log.Println("can't send chunk", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			resp := map[string]interface{}{
				"jsonrpc": "2.0",
				"result":  reply,
				"id":      req.ID,
			}

			w.WriteHeader(200)
			if err := json.NewEncoder(w).Encode(resp); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				log.Println(err)
			}

		case "das_commitChunkedStore":

			var args2 []string
			if err := json.Unmarshal(req.Params, &args2); err != nil {
				http.Error(w, "Invalid das_commitChunkedStore params", http.StatusBadRequest)
				return
			}

			var args CommitChunkedStoreArgs
			args.BatchId, err = strconv.ParseUint(args2[0][2:], 16, 64)
			if err != nil {
				log.Println("can't decode hex here", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			reply, err := service.CommitChunkedStore(&args)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			resp := map[string]interface{}{
				"jsonrpc": "2.0",
				"result":  reply,
				"id":      req.ID,
			}

			w.WriteHeader(200)
			if err := json.NewEncoder(w).Encode(resp); err != nil {
				log.Println(err)
			}

			b, err := json.MarshalIndent(resp, "", "  ")
			if err != nil {
				log.Println(err)
			}

			log.Println(string(b))
		default:
			http.Error(w, "Method not found", http.StatusNotFound)
			return
		}
	}
}

type DASServiceConfig struct {
	Enable     bool   `koanf:"enable"`
	Network    string `konaf:"network"`
	Canister   string `konaf:"canister"`
	KeysetHash string `konaf:"keysetHash"`
}

func NewDASService(config DASServiceConfig) (*DASService, error) {
	u, err := url.Parse(config.Network)
	if err != nil {
		return nil, err
	}

	aconfig := agent.Config{
		ClientConfig:                   &agent.ClientConfig{Host: u},
		FetchRootKey:                   true,
		DisableSignedQueryVerification: true,
	}

	p := principal.MustDecode(string(config.Canister))

	a, err := icutils.NewAgent(p, aconfig)
	if err != nil {
		return nil, err
	}

	return &DASService{
		Canister:   p,
		Agent:      a,
		keysetHash: config.KeysetHash,
	}, nil
}

func main() {

	config := DASServiceConfig{
		Enable: true,
	}

	config.Network = os.Getenv("DAS_NETWORK")
	if config.Network == "" {
		config.Network = "http://127.0.0.1:4943/"
	}

	config.Canister = os.Getenv("DAS_CANISTER")
	if config.Canister == "" {
		config.Canister = "bkyz2-fmaaa-aaaaa-qaaaq-cai"
	}

	config.KeysetHash = os.Getenv("DAS_KEYSET_HASH")
	if config.KeysetHash == "" {
		config.KeysetHash = "0xb2fd804a20ccbfcfcb4053db7349d066b5ce00b01a48128754d4131fd5aeb741"
	}

	service, err := NewDASService(config)
	if err != nil {
		log.Println(err)
		return
	}

	http.HandleFunc("/get-by-hash/", RESTHandler(service))
	http.HandleFunc("/rpc/", JSONRPCHandler(service))

	log.Println("Server started at :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
