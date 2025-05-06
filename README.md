# Diploma work for SPBSTU #

## Dependencies ##
- Docker desktop
- Make
- GoLang
- WSL (or not if running in Linux systems)

## How to run ##
### 1. Install all dependencies ###
### 2. Run Docker desktop ###
### 3. Run WSL ###
### 4. Open project folder in terminal ###
### 5. Build, run services (toasted postgres)
```
make toasted
```
### 5(2). Build, run services (vanilla postgres)
```
make original
```
### 6. Open another Terminal
### 7. Open monitor folder
### 8. Build monitor app ###
```
go mod init monitor && go mod tidy
```
### 9. Run monitor app ###
```
go run main.go
```
### 10. In browser open localhost:8190 ###