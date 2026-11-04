# Variáveis de Ambiente

Referência rápida de variáveis de ambiente do Evolution GO.

Para documentação detalhada, consulte: [Configuração](../fundamentos/configuration.md)

---

## Obrigatórias

| Variável | Descrição | Exemplo |
|----------|-----------|---------|
| `GLOBAL_API_KEY` | Chave de autenticação da API | `df16caad-d0d2-41b2-bec5-75b90048a0db` |
| `DATABASE_SAVE_MESSAGES` | Salvar mensagens no banco | `false` |

---

## Servidor

| Variável | Padrão | Descrição |
|----------|--------|-----------|
| `SERVER_PORT` | `4000` | Porta HTTP |
| `CLIENT_NAME` | `evolution` | Nome identificador |
| `OS_NAME` | `Linux` | Sistema operacional |

---

## Banco de Dados

| Variável | Descrição |
|----------|-----------|
| `POSTGRES_AUTH_DB` | Connection string banco de autenticação |
| `POSTGRES_USERS_DB` | Connection string banco de usuários |

**Formato:**
```env
POSTGRES_AUTH_DB=postgresql://user:pass@host:5432/evogo_auth?sslmode=disable
POSTGRES_USERS_DB=postgresql://user:pass@host:5432/evogo_users?sslmode=disable
```

---

## Logs

| Variável | Padrão | Valores | Descrição |
|----------|--------|---------|-----------|
| `WADEBUG` | `INFO` | `DEBUG`, `INFO`, `WARN`, `ERROR` | Nível de log |
| `LOGTYPE` | `console` | `console`, `file` | Destino de saída |
| `LOG_DIRECTORY` | `/app/logs` | - | Diretório de arquivos de log |
| `LOG_MAX_SIZE` | `100` | - | Tamanho máximo por arquivo (MB) |
| `LOG_MAX_BACKUPS` | `5` | - | Arquivos de backup a manter |
| `LOG_MAX_AGE` | `30` | - | Retenção em dias |
| `LOG_COMPRESS` | `true` | `true`/`false` | Compressão de logs antigos |

---

## Conexão e Comportamento

| Variável | Padrão | Descrição |
|----------|--------|-----------|
| `CONNECT_ON_STARTUP` | `true` | Ao iniciar, conecta todas as instâncias pareadas (inclusive as paradas via disconnect; use logout para impedir) |
| `WEBHOOK_FILES` | `true` | Enviar URLs de mídia em webhooks |
| `QRCODE_MAX_COUNT` | `5` | Tentativas máximas de QR Code |
| `CHECK_USER_EXISTS` | `true` | Validar destinatário antes de enviar |
| `REREQUEST_FROM_PHONE` | `false` | Pedir ao celular as mensagens que falharam ao descriptografar (evita perder a 1ª mensagem de contatos novos) |

---

## Eventos

| Variável | Padrão | Descrição |
|----------|--------|-----------|
| `EVENT_IGNORE_GROUP` | `false` | Ignorar eventos de grupos |
| `EVENT_IGNORE_STATUS` | `true` | Ignorar eventos de status/stories |
| `WEBHOOK_URL` | - | URL para callbacks HTTP |

---

## RabbitMQ (AMQP)

| Variável | Descrição |
|----------|-----------|
| `AMQP_URL` | URL de conexão RabbitMQ |
| `AMQP_GLOBAL_ENABLED` | Habilitar filas globais |
| `AMQP_GLOBAL_EVENTS` | Eventos a publicar (separados por vírgula) |
| `AMQP_SPECIFIC_EVENTS` | Eventos específicos por instância |

**Exemplo:**
```env
AMQP_URL=amqp://user:pass@rabbitmq:5672/vhost
AMQP_GLOBAL_ENABLED=true
AMQP_GLOBAL_EVENTS=messages.upsert,messages.update,connection.update
```

---

## NATS

| Variável | Descrição |
|----------|-----------|
| `NATS_URL` | URL de conexão NATS |
| `NATS_GLOBAL_ENABLED` | Habilitar publicação global |
| `NATS_GLOBAL_EVENTS` | Eventos a publicar |

**Exemplo:**
```env
NATS_URL=nats://nats:4222
NATS_GLOBAL_ENABLED=true
NATS_GLOBAL_EVENTS=messages.upsert,connection.update
```

---

## MinIO/S3

| Variável | Descrição |
|----------|-----------|
| `MINIO_ENABLED` | Habilitar armazenamento S3-compatible |
| `MINIO_ENDPOINT` | Endpoint do servidor |
| `MINIO_ACCESS_KEY` | Access Key para autenticação |
| `MINIO_SECRET_KEY` | Secret Key para autenticação |
| `MINIO_BUCKET` | Nome do bucket |
| `MINIO_USE_SSL` | Utilizar HTTPS |
| `MINIO_REGION` | Região do bucket (AWS) |

**Exemplo:**
```env
MINIO_ENABLED=true
MINIO_ENDPOINT=localhost:9000
MINIO_ACCESS_KEY=minioadmin
MINIO_SECRET_KEY=minioadmin
MINIO_BUCKET=evolution-media
MINIO_USE_SSL=false
MINIO_REGION=us-east-1
```

---

## Proxy HTTP

| Variável | Descrição |
|----------|-----------|
| `PROXY_HOST` | Hostname do proxy |
| `PROXY_PORT` | Porta do proxy |
| `PROXY_USERNAME` | Usuário (opcional) |
| `PROXY_PASSWORD` | Senha (opcional) |

**Exemplo:**
```env
PROXY_HOST=proxy.empresa.com
PROXY_PORT=8080
PROXY_USERNAME=usuario
PROXY_PASSWORD=senha
```

---

## Recursos Adicionais

| Variável | Descrição |
|----------|-----------|
| `API_AUDIO_CONVERTER` | URL de serviço de conversão de áudio |
| `API_AUDIO_CONVERTER_KEY` | Chave de autenticação do conversor |

---

## Versão WhatsApp (Avançado)

| Variável | Descrição |
|----------|-----------|
| `WHATSAPP_VERSION_MAJOR` | Versão major do WhatsApp Web |
| `WHATSAPP_VERSION_MINOR` | Versão minor do WhatsApp Web |
| `WHATSAPP_VERSION_PATCH` | Versão patch do WhatsApp Web |

**⚠️ Atenção**: Modificar versão do WhatsApp pode resultar em bloqueio. Deixar vazio para usar versão automática.

---

## Exemplo Completo

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

# Webhook
WEBHOOK_URL=https://seu-servidor.com/webhook

# RabbitMQ (opcional)
AMQP_URL=amqp://admin:admin@rabbitmq:5672/default
AMQP_GLOBAL_ENABLED=true
AMQP_GLOBAL_EVENTS=messages.upsert,connection.update

# MinIO (opcional)
MINIO_ENABLED=true
MINIO_ENDPOINT=minio:9000
MINIO_ACCESS_KEY=minioadmin
MINIO_SECRET_KEY=minioadmin
MINIO_BUCKET=evolution-media
MINIO_USE_SSL=false
```

---

## Recursos

- **[Configuração Detalhada](../fundamentos/configuration.md)** - Documentação completa de cada variável
- **[.env.example](https://git.evoai.app/Evolution/evolution-go/blob/main/docker/examples/.env.example)** - Arquivo de exemplo com todas as variáveis

---

**Documentação Evolution GO v1.0**
