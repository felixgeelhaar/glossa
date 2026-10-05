module github.com/felixgeelhaar/glossa/runtimes/go

go 1.26.5

// In this repository the runtime builds against the messageformat
// beside it. Consumers ignore this replace and resolve the required
// version, so tag messageformat/vX before runtimes/go/vX.
replace github.com/felixgeelhaar/glossa/messageformat => ../../messageformat

require (
	github.com/felixgeelhaar/glossa/messageformat v0.4.0
	go.klarlabs.de/fortify v1.10.0
	golang.org/x/text v0.40.0
)

require (
	github.com/agentable/go-intl v0.2.17 // indirect
	github.com/cockroachdb/apd/v3 v3.2.3 // indirect
	github.com/go-json-experiment/json v0.0.0-20260623181947-01eb4420fa68 // indirect
	github.com/kaptinlin/messageformat-go v0.8.6 // indirect
	github.com/kaptinlin/messageformat-go/mf1 v0.8.6 // indirect
)
