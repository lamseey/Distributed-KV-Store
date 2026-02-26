# Distributed-KV-Store

Simple distributed key-value store with a Raft-like leader election and log replication.

## Run a 3-node cluster

Open three terminals and run:

```bash
go run . -id 1 -addr localhost:8001 -peers localhost:8002,localhost:8003 -data data -compact 2m
go run . -id 2 -addr localhost:8002 -peers localhost:8001,localhost:8003 -data data -compact 2m
go run . -id 3 -addr localhost:8003 -peers localhost:8001,localhost:8002 -data data -compact 2m
```

Each node exposes RPC and HTTP on the same address.

## HTTP API

- `GET /get/{key}`
- `POST /set` with JSON body: `{ "key": "name", "value": "Gemini" }`
- `GET /cluster/status`

Examples:

```bash
Invoke-RestMethod http://localhost:8001/cluster/status
Invoke-RestMethod -Uri http://localhost:8001/set -Method Post -ContentType "application/json" -Body '{"key":"name","value":"salim"}'
Invoke-RestMethod http://localhost:8002/get/name
```

## Persistence

Each node writes its state to `data/node-{id}.json`. Remove the `data` folder to start fresh.
