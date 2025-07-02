# Makefile for the tstats project

# Go parameters
GO = go
GOBUILD = $(GO) build
GOCLEAN = $(GO) clean
GOTEST = $(GO) test

# Project parameters
BINARY_NAME = tstats
BINARY_DIR = bin
TARGET = $(BINARY_DIR)/$(BINARY_NAME)
INSTALL_PATH = /usr/local/bin

# Default target executed when you just run `make`
all: build

# Builds the binary as the current user
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BINARY_DIR)
	@$(GOBUILD) -o $(TARGET) .

# Runs the application
run: build
	@echo "Running $(BINARY_NAME)..."
	@./$(TARGET)

# Installs the binary. This target should be run with sudo, e.g., `sudo make install`
# It depends on 'build' which will be run as the user first if needed.
install: build
	@echo "Installing $(BINARY_NAME) to $(INSTALL_PATH)..."
	@sudo cp $(TARGET) $(INSTALL_PATH)/$(BINARY_NAME)
	@echo "$(BINARY_NAME) installed successfully at $(INSTALL_PATH)/$(BINARY_NAME)"

# Removes the built binary and directory
clean:
	@echo "Cleaning up..."
	@rm -rf $(BINARY_DIR)

# Uninstalls the binary from the system
uninstall:
	@echo "Uninstalling $(BINARY_NAME) from $(INSTALL_PATH)..."
	@sudo rm -f $(INSTALL_PATH)/$(BINARY_NAME)
	@echo "$(BINARY_NAME) uninstalled."


# Declare non-file targets
.PHONY: all build run install clean uninstall
