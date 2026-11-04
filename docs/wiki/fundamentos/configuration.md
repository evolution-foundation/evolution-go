# Configuração

Referência completa de variáveis de ambiente do Evolution GO.

Para exemplos práticos, consulte: `docker/examples/.env.example`

## Configuração via Ambiente

As configurações podem ser definidas através de:
- **Variáveis de ambiente** (Docker, Kubernetes)
- **Arquivo `.env`** (instalação local)

---

## Variáveis Obrigatórias

### GLOBAL_API_KEY

Chave de autenticação para acesso à API.

- **Tipo**: String (UUID recomendado)
- **Obrigatório**: Sim

```env
GLOBAL_API_KEY=df16caad-d0d2-41b2-bec5-75b90048a0db
```

**Geração:**
```bash
# UUID v4
uuidgen

# Ou via Python
python3 -c "import uuid; print(uuid.uuid4())"
```

### DATABASE_SAVE_MESSAGES  

Habilita persistência de mensagens no banco de dados.

- **Tipo**: Boolean
- **Obrigatório**: Sim
- **Padrão**: `false`

```env
DATABASE_SAVE_MESSAGES=false
```

**Nota**: `true` aumenta significativamente o uso de storage. Recomendado apenas quando necessário manter histórico completo.

---

## Servidor

### SERVER_PORT

Porta HTTP do servidor API.

- **Tipo**: Integer
- **Padrão**: `4000`

```env
SERVER_PORT=4000
```

### CLIENT_NAME

Identificador do cliente/instalação.

- **Tipo**: String

```env
CLIENT_NAME=evolution-production
```

### OS_NAME

Sistema operacional do ambiente.

- **Tipo**: String
- **Valores**: `Linux`, `Windows`, `macOS`

```env
OS_NAME=Linux
```

---

## Banco de Dados PostgreSQL

### POSTGRES_AUTH_DB

String de conexão para banco de autenticação.

- **Formato**: `postgresql://user:pass@host:port/database?sslmode=disable`

```env
POSTGRES_AUTH_DB=postgresql://postgres:senha@postgres:5432/evogo_auth?sslmode=disable
```

### POSTGRES_USERS_DB

String de conexão para banco de dados de usuários.

```env
POSTGRES_USERS_DB=postgresql://postgres:senha@postgres:5432/evogo_users?sslmode=disable
```

**Componentes da URL:**
- `postgres` - usuário
- `senha` - password (substituir)
- `postgres` - hostname (ou `localhost`)
- `5432` - porta
- `evogo_auth` / `evogo_users` - database name

---

## Logs e Debug

### WADEBUG

Nível de logging do WhatsApp.

- **Valores**: `ERROR`, `INFO`, `DEBUG`
- **Padrão**: `INFO`

```env
WADEBUG=INFO
```

- `ERROR` - Apenas erros críticos
- `INFO` - Informações importantes
- `DEBUG` - Detalhamento completo (uso em desenvolvimento)

### LOGTYPE

Destino de saída dos logs.

- **Valores**: `console`, `file`
- **Padrão**: `console`

```env
LOGTYPE=console
```

### LOG_DIRECTORY

Diretório para arquivos de log (quando `LOGTYPE=file`).

- **Padrão**: `./logs`

```env
LOG_DIRECTORY=/app/logs
```

### LOG_MAX_SIZE

Tamanho máximo de cada arquivo de log em MB.

- **Padrão**: `100`

```env
LOG_MAX_SIZE=100
```

### LOG_MAX_BACKUPS

Número de arquivos de log rotacionados a manter.

- **Padrão**: `5`

```env
LOG_MAX_BACKUPS=5
```

### LOG_MAX_AGE

Dias de retenção de arquivos de log.

- **Padrão**: `30`

```env
LOG_MAX_AGE=30
```

### LOG_COMPRESS

Compressão de logs rotacionados.

- **Tipo**: Boolean
- **Padrão**: `true`

```env
LOG_COMPRESS=true
```

---

## Conexão e Comportamento

### CONNECT_ON_STARTUP

Conecta automaticamente, ao iniciar o servidor, todas as instâncias **pareadas** (com `jid` salvo), mesmo as que estavam desconectadas — inclusive as paradas via `/instance/disconnect`. Para que uma instância não volte no próximo restart, use `/instance/logout`. Instâncias cuja sessão do WhatsApp não existe mais no banco de auth são ignoradas com o aviso "Stored WhatsApp session not found" e precisam de um QR novo.

- **Tipo**: Boolean
- **Padrão**: `true` (quando a variável não está definida)

```env
CONNECT_ON_STARTUP=false
```

**Recomendação**: `false` para maior controle sobre o ciclo de vida das instâncias.

### WEBHOOK_FILES

Envia URLs de mídia nos payloads de webhook.

- **Tipo**: Boolean
- **Padrão**: `true`

```env
WEBHOOK_FILES=true
```

### QRCODE_MAX_COUNT

Número máximo de tentativas de geração de QR Code antes de exigir reconexão manual.

- **Tipo**: Integer
- **Padrão**: `5`

```env
QRCODE_MAX_COUNT=5
```

### CHECK_USER_EXISTS

Valida existência do destinatário no WhatsApp antes de enviar mensagem.

- **Tipo**: Boolean
- **Padrão**: `true`

```env
CHECK_USER_EXISTS=true
```

**Nota**: `false` pode resultar em erros de envio para números inválidos.

### EVENT_IGNORE_GROUP

Ignora eventos originados de grupos.

- **Tipo**: Boolean
- **Padrão**: `false`

```env
EVENT_IGNORE_GROUP=false
```

### EVENT_IGNORE_STATUS

Ignora eventos de status/stories.

- **Tipo**: Boolean
- **Padrão**: `true`

```env
EVENT_IGNORE_STATUS=true
```

---

## Webhooks

### WEBHOOK_URL

URL de destino para callbacks HTTP de eventos.

```env
WEBHOOK_URL=https://api.seudominio.com/webhook
```

Eventos serão enviados via POST com payload JSON:

```json
{
  "event": "messages.upsert",
  "instanceName": "nome-instancia",
  "data": { ... }
}
```

---

## RabbitMQ (AMQP)

Sistema de filas para processamento assíncrono de eventos.

### AMQP_URL

URL de conexão RabbitMQ.

- **Formato**: `amqp://user:pass@host:port/vhost`

```env
AMQP_URL=amqp://admin:admin@rabbitmq:5672/default
```

### AMQP_GLOBAL_ENABLED

Habilita publicação global de eventos via RabbitMQ.

- **Tipo**: Boolean
- **Padrão**: `false`

```env
AMQP_GLOBAL_ENABLED=true
```

### AMQP_GLOBAL_EVENTS

Lista de eventos a serem publicados (separados por vírgula).

```env
AMQP_GLOBAL_EVENTS=messages.upsert,messages.update,connection.update
```

**Eventos disponíveis:**
- `messages.upsert` - Nova mensagem recebida
- `messages.update` - Atualização de mensagem (leitura, entrega)
- `connection.update` - Mudança de status de conexão
- Entre outros

---

## NATS

Sistema de mensageria para eventos distribuídos.

### NATS_URL

URL do servidor NATS.

```env
NATS_URL=nats://localhost:4222
```

### NATS_GLOBAL_ENABLED

Habilita publicação de eventos via NATS.

- **Tipo**: Boolean
- **Padrão**: `false`

```env
NATS_GLOBAL_ENABLED=true
```

---

## MinIO/S3

Armazenamento de objetos para mídia.

### MINIO_ENABLED

Habilita integração com MinIO/S3.

- **Tipo**: Boolean
- **Padrão**: `false`

```env
MINIO_ENABLED=true
```

### MINIO_ENDPOINT

Endpoint do servidor de objetos.

```env
# MinIO local
MINIO_ENDPOINT=localhost:9000

# AWS S3
MINIO_ENDPOINT=s3.amazonaws.com
```

### MINIO_ACCESS_KEY

Access Key para autenticação.

```env
MINIO_ACCESS_KEY=minioadmin
```

### MINIO_SECRET_KEY  

Secret Key para autenticação.

```env
MINIO_SECRET_KEY=minioadmin
```

### MINIO_BUCKET

Nome do bucket para armazenamento de mídia.

```env
MINIO_BUCKET=evolution-media
```

**Nota**: O bucket deve existir antes de habilitar a integração.

### MINIO_USE_SSL

Utiliza HTTPS para conexões ao S3/MinIO.

- **Tipo**: Boolean
- **Padrão**: `false`

```env
MINIO_USE_SSL=true
```

### MINIO_REGION

Região do bucket S3 (AWS).

- **Padrão**: `us-east-1`

```env
MINIO_REGION=us-east-1
```

---

## Proxy

Configuração de proxy HTTP para instâncias WhatsApp.

### PROXY_HOST

Hostname do servidor proxy.

```env
PROXY_HOST=proxy.empresa.com
```

### PROXY_PORT

Porta do proxy.

```env
PROXY_PORT=8080
```

### PROXY_USERNAME

Usuário para autenticação (se requerido).

```env
PROXY_USERNAME=usuario
```

### PROXY_PASSWORD

Senha para autenticação (se requerido).

```env
PROXY_PASSWORD=senha
```

---

## Recursos Adicionais

### API_AUDIO_CONVERTER

URL de serviço externo para conversão de áudio.

```env
API_AUDIO_CONVERTER=https://converter.seudominio.com
```

### API_AUDIO_CONVERTER_KEY

Chave de autenticação do serviço de conversão.

```env
API_AUDIO_CONVERTER_KEY=chave-do-servico
```

---

## Versão WhatsApp (Avançado)

**⚠️ Atenção**: Modificar a versão do WhatsApp pode resultar em bloqueio de conta. Use apenas quando estritamente necessário.

### WHATSAPP_VERSION_MAJOR

```env
WHATSAPP_VERSION_MAJOR=2
```

### WHATSAPP_VERSION_MINOR

```env
WHATSAPP_VERSION_MINOR=2412
```

### WHATSAPP_VERSION_PATCH

```env
WHATSAPP_VERSION_PATCH=54
```

**Recomendação**: Deixar não configurado para utilizar versão automática da biblioteca whatsmeow.

---

## Exemplo de Configuração Mínima

```env
# Obrigatórias
GLOBAL_API_KEY=df16caad-d0d2-41b2-bec5-75b90048a0db
DATABASE_SAVE_MESSAGES=false

# Servidor
SERVER_PORT=4000
CLIENT_NAME=evolution
OS_NAME=Linux

# Banco de Dados
POSTGRES_AUTH_DB=postgresql://postgres:senha@postgres:5432/evogo_auth?sslmode=disable
POSTGRES_USERS_DB=postgresql://postgres:senha@postgres:5432/evogo_users?sslmode=disable

# Logs
WADEBUG=INFO
LOGTYPE=console

# Comportamento
CONNECT_ON_STARTUP=false
WEBHOOK_FILES=true
CHECK_USER_EXISTS=true
EVENT_IGNORE_STATUS=true
```

---

## Referências

- **[Instalação](./installation.md)** - Guia de instalação
- **[Início Rápido](./quickstart.md)** - Primeiro uso
- **[.env.example](../../../docker/examples/.env.example)** - Exemplo completo com todas variáveis

---

**Documentação Evolution GO v1.0**
