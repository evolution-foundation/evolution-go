.PHONY: help dev run build test clean swagger deps docker-build docker-run install setup migrate-up migrate-down logs

# Configurações
APP_NAME=evolution-go
MAIN_PATH=cmd/evolution-go/main.go
BUILD_DIR=build
GO=go
VERSION=$(shell grep -om1 "v[0-9].*" CHANGELOG.md)
LDFLAGS=-ldflags "-X main.version=$(VERSION)"
GOFLAGS=-v

# Cores para output
GREEN=\033[0;32m
YELLOW=\033[0;33m
RED=\033[0;31m
NC=\033[0m # No Color

##@ Ajuda

help: ## Exibe esta mensagem de ajuda
	@echo "$(GREEN)Evolution GO - Makefile$(NC)"
	@echo ""
	@awk 'BEGIN {FS = ":.*##"; printf "\nUso:\n  make $(YELLOW)<target>$(NC)\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  $(GREEN)%-15s$(NC) %s\n", $$1, $$2 } /^##@/ { printf "\n$(YELLOW)%s$(NC)\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Desenvolvimento

dev: ## Roda a aplicação em modo desenvolvimento
	@echo "$(GREEN)🚀 Rodando Evolution GO em modo desenvolvimento...$(NC)"
	$(GO) run $(LDFLAGS) $(MAIN_PATH) -dev

run: ## Roda a aplicação em modo produção
	@echo "$(GREEN)🚀 Rodando Evolution GO...$(NC)"
	$(GO) run $(MAIN_PATH)

watch: ## Roda a aplicação com hot reload (requer air)
	@if command -v air > /dev/null; then \
		echo "$(GREEN)🔥 Rodando com hot reload...$(NC)"; \
		air; \
	else \
		echo "$(RED)❌ Air não instalado. Instale com: go install github.com/cosmtrek/air@latest$(NC)"; \
		exit 1; \
	fi

##@ Build

build: ## Compila a aplicação
	@echo "$(GREEN)🔨 Compilando $(APP_NAME)...$(NC)"
	@mkdir -p $(BUILD_DIR)
	$(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(APP_NAME) $(MAIN_PATH)
	@echo "$(GREEN)✅ Build completo: $(BUILD_DIR)/$(APP_NAME)$(NC)"

build-linux: ## Compila para Linux
	@echo "$(GREEN)🔨 Compilando para Linux...$(NC)"
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(APP_NAME)-linux-amd64 $(MAIN_PATH)
	@echo "$(GREEN)✅ Build Linux completo$(NC)"

build-windows: ## Compila para Windows
	@echo "$(GREEN)🔨 Compilando para Windows...$(NC)"
	@mkdir -p $(BUILD_DIR)
	GOOS=windows GOARCH=amd64 $(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(APP_NAME)-windows-amd64.exe $(MAIN_PATH)
	@echo "$(GREEN)✅ Build Windows completo$(NC)"

build-all: build build-linux build-windows ## Compila para todas as plataformas
	@echo "$(GREEN)✅ Todos os builds completos$(NC)"

install: build ## Compila e instala no GOPATH
	@echo "$(GREEN)📦 Instalando $(APP_NAME)...$(NC)"
	$(GO) install $(MAIN_PATH)
	@echo "$(GREEN)✅ Instalado com sucesso$(NC)"

##@ Testes

test: ## Roda todos os testes
	@echo "$(GREEN)🧪 Rodando testes...$(NC)"
	$(GO) test -v ./...

test-coverage: ## Roda testes com cobertura
	@echo "$(GREEN)🧪 Rodando testes com cobertura...$(NC)"
	$(GO) test -v -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "$(GREEN)✅ Cobertura gerada: coverage.html$(NC)"

test-race: ## Roda testes verificando race conditions
	@echo "$(GREEN)🧪 Rodando testes com race detector...$(NC)"
	$(GO) test -race -v ./...

bench: ## Roda benchmarks
	@echo "$(GREEN)⚡ Rodando benchmarks...$(NC)"
	$(GO) test -bench=. -benchmem ./...

##@ Dependências

deps: ## Instala dependências
	@echo "$(GREEN)📦 Instalando dependências...$(NC)"
	$(GO) mod download
	$(GO) mod verify
	@echo "$(GREEN)✅ Dependências instaladas$(NC)"

deps-update: ## Atualiza dependências
	@echo "$(GREEN)📦 Atualizando dependências...$(NC)"
	$(GO) get -u ./...
	$(GO) mod tidy
	@echo "$(GREEN)✅ Dependências atualizadas$(NC)"

deps-clean: ## Limpa dependências não utilizadas
	@echo "$(GREEN)🧹 Limpando dependências...$(NC)"
	$(GO) mod tidy
	@echo "$(GREEN)✅ Dependências limpas$(NC)"

deps-reset: ## Limpa cache e reinstala dependências (força uso do código local)
	@echo "$(GREEN)🔄 Resetando dependências e cache...$(NC)"
	@echo "$(YELLOW)Limpeza de cache e módulos...$(NC)"
	$(GO) clean -cache -modcache -i -r
	@echo "$(YELLOW)Download de módulos...$(NC)"
	$(GO) mod download
	@echo "$(YELLOW)Organizando módulos...$(NC)"
	$(GO) mod tidy
	@echo "$(GREEN)✅ Dependências resetadas e atualizadas$(NC)"

##@ Documentação

swagger: ## Gera documentação Swagger
	@echo "$(GREEN)📚 Gerando documentação Swagger...$(NC)"
	@if command -v swag > /dev/null; then \
		swag init -g $(MAIN_PATH) -o ./docs --parseDependency --parseInternal && \
		echo "$(GREEN)✅ Swagger gerado com sucesso$(NC)"; \
	else \
		echo "$(RED)❌ Swag não instalado. Instale com: go install github.com/swaggo/swag/cmd/swag@v1.16.3$(NC)"; \
		exit 1; \
	fi

docs: ## Abre a documentação local
	@echo "$(GREEN)📖 Abrindo documentação...$(NC)"
	@if [ -f "docs/wiki/README.md" ]; then \
		echo "Documentação disponível em: docs/wiki/README.md"; \
	else \
		echo "$(RED)❌ Documentação não encontrada$(NC)"; \
	fi

##@ Database

migrate-up: ## Executa migrations do banco de dados
	@echo "$(GREEN)🗃️  Executando migrations...$(NC)"
	@if [ -d "migrations" ]; then \
		$(GO) run $(MAIN_PATH) migrate up; \
	else \
		echo "$(YELLOW)⚠️  Diretório migrations não encontrado$(NC)"; \
	fi

migrate-down: ## Reverte migrations do banco de dados
	@echo "$(YELLOW)⚠️  Revertendo migrations...$(NC)"
	@if [ -d "migrations" ]; then \
		$(GO) run $(MAIN_PATH) migrate down; \
	else \
		echo "$(YELLOW)⚠️  Diretório migrations não encontrado$(NC)"; \
	fi

##@ Docker

docker-build: ## Build da imagem Docker
	@echo "$(GREEN)🐳 Construindo imagem Docker...$(NC)"
	docker build --build-arg VERSION=$(VERSION) -t $(APP_NAME):latest .
	@echo "$(GREEN)✅ Imagem Docker construída$(NC)"

docker-run: ## Roda container Docker
	@echo "$(GREEN)🐳 Iniciando container...$(NC)"
	docker run -p 4000:4000 --env-file .env $(APP_NAME):latest

docker-compose-up: ## Sobe todos os serviços com docker-compose
	@echo "$(GREEN)🐳 Iniciando serviços com docker-compose...$(NC)"
	docker-compose up -d

docker-compose-down: ## Para todos os serviços do docker-compose
	@echo "$(YELLOW)🐳 Parando serviços...$(NC)"
	docker-compose down

docker-compose-logs: ## Exibe logs do docker-compose
	docker-compose logs -f

##@ Linting e Formatação

fmt: ## Formata o código
	@echo "$(GREEN)✨ Formatando código...$(NC)"
	$(GO) fmt ./...
	@echo "$(GREEN)✅ Código formatado$(NC)"

lint: ## Executa linter (requer golangci-lint)
	@echo "$(GREEN)🔍 Executando linter...$(NC)"
	@if command -v golangci-lint > /dev/null; then \
		golangci-lint run ./...; \
		echo "$(GREEN)✅ Lint completo$(NC)"; \
	else \
		echo "$(RED)❌ golangci-lint não instalado. Instale com: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest$(NC)"; \
		exit 1; \
	fi

vet: ## Executa go vet
	@echo "$(GREEN)🔍 Executando go vet...$(NC)"
	$(GO) vet ./...
	@echo "$(GREEN)✅ Vet completo$(NC)"

check: fmt vet lint test ## Executa todas as verificações

##@ Limpeza

clean: ## Remove arquivos de build
	@echo "$(YELLOW)🧹 Limpando arquivos de build...$(NC)"
	@rm -rf $(BUILD_DIR)
	@rm -f coverage.out coverage.html
	@echo "$(GREEN)✅ Limpeza completa$(NC)"

clean-all: clean ## Remove arquivos de build e cache
	@echo "$(YELLOW)🧹 Limpeza completa (incluindo cache)...$(NC)"
	$(GO) clean -cache -testcache -modcache
	@echo "$(GREEN)✅ Limpeza completa$(NC)"

##@ Utilitários

setup: deps swagger ## Setup completo do ambiente de desenvolvimento
	@echo "$(GREEN)🎉 Setup completo!$(NC)"
	@echo ""
	@echo "Para começar a desenvolver, rode:"
	@echo "  $(YELLOW)make dev$(NC)"
	@echo ""
	@echo "Outros comandos úteis:"
	@echo "  $(YELLOW)make help$(NC)       - Ver todos os comandos"
	@echo "  $(YELLOW)make test$(NC)       - Rodar testes"
	@echo "  $(YELLOW)make build$(NC)      - Compilar a aplicação"

logs: ## Exibe logs da aplicação (se estiver rodando)
	@echo "$(GREEN)📋 Exibindo logs...$(NC)"
	@if [ -f "logs/app.log" ]; then \
		tail -f logs/app.log; \
	else \
		echo "$(YELLOW)⚠️  Arquivo de log não encontrado$(NC)"; \
	fi

version: ## Exibe versão do Go e dependências
	@echo "$(GREEN)📌 Versões:$(NC)"
	@$(GO) version
	@echo ""
	@echo "$(GREEN)Dependências principais:$(NC)"
	@$(GO) list -m all | grep -E '(whatsmeow|postgres|minio)'

status: ## Verifica status da aplicação
	@echo "$(GREEN)🔍 Verificando status...$(NC)"
	@curl -s http://localhost:4000/health || echo "$(RED)❌ Aplicação não está rodando$(NC)"

##@ Desenvolvimento Avançado

profile-cpu: ## Profile de CPU (requer aplicação rodando)
	@echo "$(GREEN)📊 Capturando profile de CPU...$(NC)"
	curl http://localhost:4000/debug/pprof/profile?seconds=30 > cpu.prof
	$(GO) tool pprof -http=:8080 cpu.prof

profile-mem: ## Profile de memória (requer aplicação rodando)
	@echo "$(GREEN)📊 Capturando profile de memória...$(NC)"
	curl http://localhost:4000/debug/pprof/heap > mem.prof
	$(GO) tool pprof -http=:8080 mem.prof

generate: ## Roda go generate
	@echo "$(GREEN)⚙️  Executando go generate...$(NC)"
	$(GO) generate ./...

mod-graph: ## Exibe gráfico de dependências
	@echo "$(GREEN)📊 Gráfico de dependências:$(NC)"
	$(GO) mod graph
