export GOTOOLCHAIN=local

.PHONY help
help: ## list available targets
	@# Shamelessly stolen from Gomega's Makefile
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-16s\033[0m %s\n", $$1, $$2}'

.PHONY clean
clean: ## cleans up build and testing artefacts
	rm -f coverage.*

.PHONY test
test: ## runs unit tests
	go test -v -p=1 -count=1 -race ./...
