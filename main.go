package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"
)

// NodeConfig represents the structure of nodes configuration
type NodeConfig struct {
	Nodes      map[string]Node   `json:"nodes" yaml:"nodes"`
	PublicApis map[string]string `json:"public_apis" yaml:"public_apis"`
}

// Node represents the structure of a node configuration
type Node struct {
	Service   string `json:"service" yaml:"service"`
	Port      int    `json:"port" yaml:"port"`
	RPCPath   string `json:"rpc_path" yaml:"rpc_path"`
	Namespace string `json:"namespace" yaml:"namespace"`
}

// Result represents the structure of a node result
type Result struct {
	SyncStatus     string
	NodeBlockNum   int64
	LatestBlockNum int64
	Diff           int64
	PeersCount     int64
}

func main() {
	if len(os.Args) > 2 {
		fmt.Println("Usage: checknode <eth|bsc|arb|poly>")
		os.Exit(1)
	}

	all := len(os.Args) == 1
	chainName := ""
	if !all {
		chainName = os.Args[1]
	}

	// Read nodes configuration
	config, err := readConfig()
	if err != nil {
		fmt.Println("Error reading configuration:", err)
		os.Exit(1)
	}

	// Get node info from config
	nodes := make(map[string]Node)
	switch all {
	case true:
		nodes = config.Nodes
	case false:
		if node, ok := config.Nodes[chainName]; ok {
			nodes[chainName] = node
		} else {
			fmt.Println("Node not found in configuration")
			os.Exit(1)
		}
	default:
		fmt.Println("Invalid node name")
		os.Exit(1)
	}

	// Create a wait group to ensure all port forwards are removed
	var wg sync.WaitGroup
	localPortCounter := 1
	results := make(map[string]Result, 0)

	// Iterate over nodes and perform checks
	for nodeName, node := range nodes {
		wg.Add(1)

		lp := 8080
		if all {
			lp += localPortCounter
			localPortCounter++
		}

		go func(nodeName string, node Node, localPort int) {
			defer wg.Done()

			// Port forward
			portForwardCmd := exec.Command("kubectl", "port-forward", fmt.Sprintf("service/%s", node.Service), fmt.Sprintf("%d:%d", localPort, node.Port), "--namespace", "blockchains")
			stderr, err := portForwardCmd.StderrPipe()
			if err != nil {
				fmt.Printf("Error creating stderr pipe for %s: %v\n", nodeName, err)
				return
			}
			if err := portForwardCmd.Start(); err != nil {
				fmt.Printf("Error starting port forward for %s: %v\n", nodeName, err)
				return
			}

			// Read and print stderr in a separate goroutine
			go func() {
				scanner := bufio.NewScanner(stderr)
				for scanner.Scan() {
					fmt.Printf("Port Forwarding Error for %s: %s\n", nodeName, scanner.Text())
				}
			}()

			time.Sleep(time.Second * 3)

			defer func() {
				// Remove port forward
				removePortForwardCmd := exec.Command("killall", "kubectl")
				removePortForwardCmd.Run()
			}()

			// RPC endpoints
			status, err := callRPC(node, localPort, "eth_syncing")
			if err != nil {
				fmt.Printf("Error getting sync status for %s: %v\n", nodeName, err)
				return
			}

			peersCountNum := int64(0)
			if nodeName != "arb" {
				peersCount, err := callRPC(node, localPort, "net_peerCount")
				if err != nil {
					fmt.Printf("Error getting peers count for %s: %v\n", nodeName, err)
					return
				}
				peersCountNum, err = strconv.ParseInt(peersCount.(string)[2:], 16, 64)
				if err != nil {
					fmt.Printf("Error getting peers count for %s: %v\n", nodeName, err)
					return
				}
			}

			currentNodeBlock, err := callRPC(node, localPort, "eth_blockNumber")
			if err != nil {
				fmt.Printf("Error getting latest block for %s: %v\n", nodeName, err)
				return
			}
			currentNodeBlockNum, err := strconv.ParseInt(currentNodeBlock.(string)[2:], 16, 64)
			if err != nil {
				fmt.Printf("Error getting latest block for %s: %v\n", nodeName, err)
				return
			}

			latestBlock, err := fetchLatestBlock(nodeName, config.PublicApis[nodeName])
			if err != nil {
				fmt.Printf("Error getting latest block from scanner for %s: %v\n", nodeName, err)
				return
			}

			// Get sync status
			syncStatus, err := getSyncStatus(status, latestBlock)
			if err != nil {
				fmt.Printf("failed to determine node %s sync status: %s\n", nodeName, err.Error())
			}

			results[nodeName] = Result{
				SyncStatus:     syncStatus,
				NodeBlockNum:   currentNodeBlockNum,
				LatestBlockNum: latestBlock,
				Diff:           latestBlock - currentNodeBlockNum,
				PeersCount:     peersCountNum,
			}
		}(nodeName, node, lp)
	}

	wg.Wait()

	for nodeName, res := range results {
		// Print results
		fmt.Printf("Node: %s\n", nodeName)
		fmt.Printf("Sync status: %s\n", res.SyncStatus)
		fmt.Printf("Node block number: %d\n", res.NodeBlockNum)
		fmt.Printf("Scanner block number: %d\n", res.LatestBlockNum)
		fmt.Printf("Diff with mainnet: %d\n", res.Diff)
		if nodeName != "arb" {
			fmt.Printf("Peers count: %d\n", res.PeersCount)
		}
		fmt.Println()
	}
}

func readConfig() (NodeConfig, error) {
	// Read config file
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return NodeConfig{}, err
	}
	configPath := homeDir + "/bin/nodes_conf.yaml"
	configFile, err := ioutil.ReadFile(configPath)
	if err != nil {
		return NodeConfig{}, err
	}

	// Unmarshal config
	var config NodeConfig
	err = yaml.Unmarshal(configFile, &config)
	if err != nil {
		return NodeConfig{}, err
	}

	return config, nil
}

func callRPC(node Node, localPort int, method string) (interface{}, error) {
	rpcURL := fmt.Sprintf("http://127.0.0.1:%d%s", localPort, node.RPCPath)
	payload := []byte(fmt.Sprintf(`{"jsonrpc": "2.0", "method": "%s", "params": [], "id": 1}`, method))

	req, err := http.NewRequest("POST", rpcURL, bytes.NewBuffer(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var result map[string]interface{}
	err = json.Unmarshal(body, &result)
	if err != nil {
		return "", err
	}

	return result["result"], nil
}

func fetchLatestBlock(nodeName string, apiUrl string) (int64, error) {
	// Make HTTP GET request to the Etherscan API
	resp, err := http.Get(apiUrl + "?module=proxy&action=eth_blockNumber")
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	// Read response body
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	// Unmarshal response JSON
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return 0, err
	}

	// Check if result contains a valid block number
	if result["result"] == nil {
		return 0, errors.New("no block number found in response")
	}

	// Return the latest block number
	return strconv.ParseInt(result["result"].(string)[2:], 16, 64)
}

func getSyncStatus(statusObject interface{}, latestBlock int64) (string, error) {
	switch val := statusObject.(type) {
	case bool:
		return "synced", nil
	case map[string]interface{}:
		if _, ok := val["startingBlock"].(string); !ok {
			return "unknown", nil
		}

		startingBlockNum, err := strconv.ParseInt(val["startingBlock"].(string)[2:], 16, 64)
		if err != nil {
			return "unknown", err
		}

		// If the difference between the current block and the starting block is less than 20,
		// consider it as syncing.
		if latestBlock-startingBlockNum > 20 {
			return "syncing", nil
		}
		return "synced", nil
	default:
		return "unknown", nil
	}
}
