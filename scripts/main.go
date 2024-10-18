package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/aviate-labs/agent-go"
)

type KeysetInfo struct {
	Keyset Keyset `json:"keyset"`
}
type Backend struct {
	URL    string `json:"url"`
	Pubkey string `json:"pubkey"`
}
type Keyset struct {
	AssumedHonest int       `json:"assumed-honest"`
	Backends      []Backend `json:"backends"`
}

func SerializeRootKey() {
	icUrl, err := url.Parse("http://localhost:4943/")
	if err != nil {
		panic(err)
	}
	c := agent.NewClient(agent.ClientConfig{
		Host: icUrl,
	})

	s, err := c.Status()
	if err != nil {
		panic(err)
	}

	rootKey := base64.StdEncoding.EncodeToString(s.RootKey)

	icdaserverUrl, err := url.Parse("http://localhost:8080/")
	if err != nil {
		panic(err)
	}

	ks := KeysetInfo{
		Keyset: Keyset{
			AssumedHonest: 1,
			Backends: []Backend{{
				URL:    icdaserverUrl.String(),
				Pubkey: rootKey,
			},
			},
		},
	}

	jsonData, err := json.MarshalIndent(ks, "", "  ")
	if err != nil {
		fmt.Println("Error marshalling data:", err)
		return
	}

	filePath := filepath.Join("/", "generated", "keyset.json")

	dir := filepath.Dir(filePath)
    if err := os.MkdirAll(dir, 0755); err != nil {
        fmt.Println("Error creating directory:", err)
        return
    }
	
	file, err := os.Create(filePath)
	if err != nil {
		fmt.Println("Error creating file:", err)
		return
	}
	defer file.Close()

	_, err = file.Write(jsonData)
	if err != nil {
		fmt.Println("Error writing to file:", err)
		return
	}

}

func main() {
	SerializeRootKey()
}
