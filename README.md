# 💢 Balansir

<!-- <p align="center">
    <img src="content/assets/balansir_small.png" alt="Balansir" title="Balansir" />
</p> -->

Layer 7 load balancer with simple configuration

**Heavily WIP. Do not use it in production in any case.**

**Documentaion coming at the end of December**

## Roadmap to near future (in no particular order)
- [x] Add config hot reload
- [x] Check HTTPS stability
- [ ] Add benchmark testing
- [x] Add Letsencrypt integration
- [x] Add rate limiting
- [ ] Add gzip feature
- [ ] Add cache algorithms
- [ ] Add dashboard (web UI with Balansir metrics)
- [ ] Add intelligible comments to exported functions and types

<!-- ## Configuration table

| Key name                 | Available values                                                                                         | Description                                                                                                                                                           |   |   |
|--------------------------|----------------------------------------------------------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------|---|---|
| `"server_list"`          |                                                                                                          | Array of strings. Accepts server endpoints without protocol. May include port.                                                                                        |   |   |
| `"ecosystem_protocol"`   | [`"http"`], [`"https"`]                                                                                      | String. Accepts protocol type for the whole ecosystem of endpoints, meaning all your servers **and** **Balansir** itself must be utilized in the same network protocol.   |   |   |
| `"load_balancer_port"`   |                                                                                                          | Integer. Accepts port for **Balansir**.                                                                                                                                   |   |   |
| `"server_check_timeout"` |                                                                                                          | Integer. Define how many seconds **Balansir** should keep connection to one of endpoints waiting for response, before it will be marked as dead until next servers check. |   |   |
| `"proxy_mode"`           | [`"transparent"`], [`"non-transparent"`]                                                                 | String. Define what proxy mode will be used within **Balansir**.                                                                                                          |   |   |
| `"balancing_algorithm"`  | [`"round-robin"`], [`"weighted-round-robin"`], [`"least-connections"`], [`"weighted-least-connections"`] | String. Define what balancing algorithm **Balansir** should utilize.                                                                                                      |   |   |

[`"http"`]: #
[`"https"`]: #
[`"transparent"`]: #
[`"non-transparent"`]: #
[`"round-robin"`]: #
[`"weighted-round-robin"`]: #
[`"least-connections"`]: #
[`"weighted-least-connections"`]: # -->
