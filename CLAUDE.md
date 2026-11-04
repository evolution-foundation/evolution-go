# CLAUDE.md

Fork pessoal (`brunonakai/evolution-go`) do [Evolution Go](https://github.com/evolution-foundation/evolution-go) — API REST em Go para WhatsApp sobre `go.mau.fi/whatsmeow`.

## Regras do fork

- **Licença**: não remover nem alterar a validação de licença (`pkg/core/c0.go`) nem os créditos/marca exigidos em `LICENSE`, `NOTICE` e `TRADEMARKS.md`. `pkg/core/c0.go` não segue `gofmt` no oficial — não reformatar.
- **Branches**:
  - `upstream` — espelho fiel de `evolution-foundation/evolution-go@main`. Nunca commitar nela.
  - `main` — estável, o que roda em produção.
  - Uma branch por correção (`fix/...`, `feat/...`, `chore/...`), sempre via PR para `main`.
- **Forks de terceiros** (ex.: keven-trabalho, Aryzonna, cateim, mira-dev-tech): servem de referência. Revisar o diff antes; nunca fazer merge/cherry-pick às cegas.
- **`.github/workflows/`**: não alterar sem pedido explícito do dono.
- **Segredos**: não ler nem exibir o conteúdo de arquivos `.env` (nem colunas `token` do banco).

## Sincronizar com o oficial

```bash
git fetch upstream
git push origin upstream/main:upstream          # atualiza o espelho
git checkout -b chore/sync-upstream-<versão> main
git merge upstream/main                          # resolver conflitos, rodar build/vet/test
# abrir PR para main
```

## Build e testes

Requer Go 1.26+ e CGO (libjpeg-turbo e libwebp). Sem Go local, usar container:

```bash
docker run --rm -v "$PWD":/src -v evogo-gomod:/go/pkg/mod -w /src golang:1.26-alpine sh -c \
  'apk add -q build-base libjpeg-turbo-dev libwebp-dev git && go build ./cmd/evolution-go && go vet ./... && go test ./...'
```

Vulnerabilidades: `go run golang.org/x/vuln/cmd/govulncheck@latest ./...` (com o patch mais recente do Go — a stdlib também é verificada). Ao subir a versão mínima no `go.mod`, atualizar `go-version` em `ci.yml`. O CI (`.github/workflows/ci.yml`) roda build, vet, testes com `-race`, govulncheck e o build da imagem Docker.

## Estrutura

- `cmd/evolution-go/main.go` — bootstrap (config, bancos, rotas, `ConnectOnStartup`).
- `pkg/whatsmeow/service/whatsmeow.go` — ciclo de vida dos clientes WhatsApp (`StartClient`, eventos, versão do WhatsApp Web).
- `pkg/<domínio>/{handler,service,repository,model}` — módulos da API (instance, sendMessage, message, group, user, ...).
- `pkg/routes/routes.go` — rotas; `pkg/middleware` — auth via header `apikey` (admin = `GLOBAL_API_KEY`, demais = token da instância).
- `pkg/config` — variáveis de ambiente (ver `.env.example`).
- `manager/dist` — frontend compilado (servido em `/manager`).

## Pontos sensíveis

- **405 "Client outdated"**: o handshake usa `store.GetWAVersion()` do whatsmeow, não `DeviceProps.Version`. `applyWhatsAppHandshakeVersion` aplica a versão do WhatsApp Web via `store.SetWAVersion` em `StartClient`. Ao atualizar o whatsmeow, conferir se isso continua valendo.
- Atualizações do whatsmeow costumam quebrar a API (ex.: `GetBlocklist`/`UpdateBlocklist` ganharam `dhash`, `SetStatusMessage` passou a receber `types.SetStatusInput`). Sempre rodar o build completo.
- `POST /instance/connect` só altera os campos presentes no JSON (`webhookUrl`, `subscribe`, `rabbitmqEnable`, `natsEnable`, `websocketEnable`); omitidos mantêm o valor salvo, e `"webhookUrl": ""` explícito limpa o webhook (`applyConnectSettings`).
- `CONNECT_ON_STARTUP` (padrão `true` no código quando a variável não está definida; `true` também no nosso compose) reconecta todas as instâncias pareadas (`jid` preenchido), via upstream PR #154 — inclusive as paradas via `/instance/disconnect`; só `/instance/logout` impede. Sessão ausente no `evogo_auth` → "Stored WhatsApp session not found", precisa de QR.
- Estouro de conexões no Postgres: existe **um único** `sqlstore.Container` por processo (`getContainer`), montado sobre o pool do `authDB` (limite em `cmd/evolution-go/main.go`). **Nunca** chamar `container.Close()` por instância: isso fecha o `authDB` de todo mundo. Quedas de conexão reaproveitam o cliente, sem recriá-lo (PR #154).

## Deploy (rmserver)

`/opt/containers/evolution-go`: `docker-compose.yml` (build a partir de `./src`), `.env` do app, `postgres.env` e `postgres-data/`. Postgres próprio (`evolution-postgres`) em rede interna; app na rede `rm-network` atrás do Traefik.

```bash
cd /opt/containers/evolution-go
git -C src checkout main && git -C src pull --ff-only
docker compose build --build-arg VERSION=$(git -C src describe --tags --always) && docker compose up -d
```
