### Concept
Layer 7 load balancer with proxy mode

### Configuration explained
`"server_list"` — array of strings. Accepts server endpoints without protocol. May include port.

Example:

```
"server_list": ["127.0.0.1:5000", "127.0.0.1:5001", "127.0.0.1:5002", "example.com", "google.com:80"]
```


`"ecosystem_protocol"` — string. Accepts protocol type for the whole ecosystem of endpoints, meaning all your servers **and** balansir itself must be utilized in the same network protocol.

Available values: 

`"http"`
`"https"`


`"load_balancer_port"` — integer. Accepts port for balansir.

Example:

```
"load_balancer_port": 8080
```


`"server_check_timeout"` — integer. Define how many seconds balansir should keep connection to one of endpoints waiting for response, before it will be marked as dead until next servers check.

Example:

```
"server_check_timeout": 3
```


`"proxy_mode"` — string. Define what proxy mode will be used within balansir.

Available values:

`"transparent"` — balansir will keep client IP address through the whole flow.

`"non-transparent"` — balansir will replace client IP address with own IP address. In this case your endpoints will receive all requests from balansir IP only.


`"balancing_algorithm"` — string. Define what balancing algorithm balansir should utilize.

Available values:

`"round-robin"` — balansir will evenly distribute requests to your endpoints.
"weighted-round-robin" — balansir will distribute requests to your endpoints based on specified weight for each such endpoint.

`"least-connections"` – balansir will distribute requests to the endpoint with least open connections. Note that least connections doen't mean least loaded server, balansir know nothing about your server's load.

`"weighted-least-connections"` — balansir will distribute requests to the endpoint with least open connections based on specified weight for each such endpoint. Priority: most weight -> least open connections
