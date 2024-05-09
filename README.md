# LL Data Availability Proof Example

This example prints the data availability proof a celestia pointer
within a given rblock. 

```
go run main.go > proof.json
```

# Environment 

The system will default to using free public nodes to connect to Ethereum and Celestia, but you can set your custom nodes.

```
export CELESTIA_TRPC="<address>:<port>"
export ETHEREUM_RPC="<your-rpc>"
```