BINARY_NAME := activitytracker
BUILD_DIR   := bin
CMD_DIR     := cmd/activitytracker

.PHONY: build test lint run clean

build:
	go build -o $(BUILD_DIR)/$(BINARY_NAME) ./$(CMD_DIR)

TEST_PKGS := \
	./internal/autostart/... \
	./internal/config/... \
	./internal/monitor/classifier/... \
	./internal/monitor/collector/... \
	./internal/report/... \
	./internal/storage/...

test:
	go test $(TEST_PKGS)

lint:
	go vet ./...

run: build
	./$(BUILD_DIR)/$(BINARY_NAME)

clean:
	rm -rf $(BUILD_DIR)
