# Architecture

This document describes the internal architecture of `gctx`.

## Directory Structure

```
gctx/
├── cmd/
│   ├── root.go              # Root command setup
│   ├── create.go            # Create account command
│   ├── save.go              # Save ADC command
│   ├── switch.go            # Switch account command
│   ├── list.go              # List accounts command
│   ├── active.go            # Show active account
│   ├── delete.go            # Delete account command
│   ├── run.go               # Run with account command
│   └── info.go              # Show account info
├── pkg/
│   ├── manager/
│   │   └── manager.go       # Core account management
│   ├── config/
│   │   └── config.go        # Configuration handling
│   ├── adc/
│   │   └── adc.go           # ADC operations
│   ├── gcloud/
│   │   └── gcloud.go        # gcloud CLI wrapper
│   └── watcher/
│       └── watcher.go       # File watching for auto-save
├── main.go                  # Entry point
├── go.mod
└── README.md
```

## Core Components

### 1. Configuration Management (`pkg/config/config.go`)

**Purpose**: Store and manage account metadata

**Storage Location**: `~/.config/gctx/config.json`

**Structure**:
```go
type Config struct {
    Accounts      map[string]*Account `json:"accounts"`
    ActiveAccount string              `json:"active_account,omitempty"`
}

type Account struct {
    Name       string    `json:"name"`
    ConfigName string    `json:"config_name"`
    ProjectID  string    `json:"project_id"`
    ADCPath    string    `json:"adc_path"`
    CreatedAt  time.Time `json:"created_at"`
    Email      string    `json:"email,omitempty"`
}
```

### 2. ADC Management (`pkg/adc/adc.go`)

**Purpose**: Handle Application Default Credentials file operations

**Storage**: `~/.config/gctx/adc/`

**Key Functions**:
*   `SaveADC(accountName)`: Copies the default ADC file to the account's storage path.
*   `RestoreADC(accountName)`: Restores the saved ADC file to the default location.
*   `ValidateADC(path)`: Ensures the ADC file is valid JSON before saving/restoring.

### 3. GCloud Integration (`pkg/gcloud/gcloud.go`)

**Purpose**: Wrapper around `gcloud` CLI commands

**Key Functions**:
*   `CreateConfig(configName)`: Creates a new gcloud configuration.
*   `ActivateConfig(configName)`: Switches the active gcloud configuration.
*   `SetProject(projectID)`: Sets the project for the current configuration.
*   `AuthLogin()`: Runs `gcloud auth login`.
*   `AuthADCLogin()`: Runs `gcloud auth application-default login`.

### 4. File Watcher (`pkg/watcher/watcher.go`)

**Purpose**: Watch ADC file for changes during auto-save flow. It detects when the authentication process updates the credentials file.

### 5. Manager (`pkg/manager/manager.go`)

**Purpose**: High-level orchestration that ties everything together.

**Key Workflows**:
*   **CreateAccount**: Creates gcloud config, sets project, and optionally triggers auto-save.
*   **SwitchAccount**: Restores the account's ADC file and activates its gcloud config.
*   **AutoSaveFlow**: Runs auth commands, watches for file changes, and saves the new credentials.

## Security Considerations

1.  **File Permissions**: ADC files are stored with restricted permissions (usually 0600) to prevent unauthorized access.
2.  **Atomic Operations**: File operations (like restoring ADC) use temporary files and atomic renames where possible to prevent corruption.
3.  **Validation**: ADC files are validated as JSON before being processed.
