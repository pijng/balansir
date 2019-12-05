# 💢 Balansir

Simple layer 7 load balancer with flexible configuration

**Heavily WIP**

## Configuration table

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
[`"weighted-least-connections"`]: #