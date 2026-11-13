# API de Instâncias

Documentação completa dos endpoints para gerenciar instâncias WhatsApp.

## Visão Geral

Uma **instância** representa uma conexão individual com o WhatsApp. Cada instância mantém sua própria sessão, autenticação e configurações independentes.

**Importante**: A maioria das rotas identifica a instância automaticamente através do header `apikey` (token da instância). Apenas rotas administrativas usam `:instanceId` na URL e exigem a `GLOBAL_API_KEY` configurada no ambiente.

## Endpoints Disponíveis

### Gerenciamento Básico (Administrativo - usa GLOBAL_API_KEY)
- `POST /instance/create` - Criar nova instância
- `GET /instance/all` - Listar todas as instâncias
- `GET /instance/info/:instanceId` - Informações da instância
- `DELETE /instance/delete/:instanceId` - Deletar instância

### Conexão (usa token da instância)
- `POST /instance/connect` - Conectar via QR Code
- `GET /instance/qr` - Obter QR Code
- `POST /instance/pair` - Conectar via código de pareamento
- `GET /instance/status` - Status da conexão
- `POST /instance/disconnect` - Desconectar
- `POST /instance/reconnect` - Reconectar
- `DELETE /instance/logout` - Fazer logout

### Gerenciamento Avançado (Administrativo - usa GLOBAL_API_KEY)
- `POST /instance/proxy/:instanceId` - Configurar proxy
- `DELETE /instance/proxy/:instanceId` - Remover proxy
- `POST /instance/forcereconnect/:instanceId` - Forçar reconexão
- `GET /instance/logs/:instanceId` - Obter logs
- `GET /instance/:instanceId/advanced-settings` - Configurações avançadas
- `PUT /instance/:instanceId/advanced-settings` - Atualizar configurações

---

## Criar Instância

Cria uma nova instância WhatsApp.

### Endpoint
```
POST /instance/create
```

### Headers
```
Content-Type: application/json
apikey: SUA-GLOBAL-API-KEY
```

**⚠️ Atenção**: Esta rota usa a `GLOBAL_API_KEY` (variável de ambiente), não o token da instância.

### Body
```json
{
  "name": "vendas",
  "token": "token-seguro-unico",
  "webhook": "https://seu-servidor.com/webhook",
  "webhookEvents": ["messages.upsert", "connection.update"],
  "proxy": {
    "host": "proxy.exemplo.com",
    "port": "8080",
    "username": "usuario",
    "password": "senha"
  }
}
```

### Parâmetros

| Campo | Tipo | Obrigatório | Descrição |
|-------|------|-------------|-----------|
| name | string | ✅ Sim | Nome único da instância |
| token | string | ✅ Sim | Token de autenticação da instância |
| webhook | string | ❌ Não | URL para receber webhooks |
| webhookEvents | array | ❌ Não | Eventos específicos para webhook |
| proxy | object | ❌ Não | Configuração de proxy |

### Resposta Sucesso (200)
```json
{
  "message": "success",
  "data": {
    "name": "vendas",
    "token": "token-seguro-unico",
    "webhook": "https://seu-servidor.com/webhook",
    "status": "created",
    "createdAt": "2025-01-15T10:30:00Z"
  }
}
```

### Resposta Erro (400)
```json
{
  "error": "name is required"
}
```

### Exemplo cURL
```bash
curl -X POST http://localhost:4000/instance/create   -H "Content-Type: application/json"   -H "apikey: SUA-CHAVE-API"   -d '{
    "name": "vendas",
    "token": "meu-token-seguro"
  }'
```

---

## Conectar Instância

Inicia o processo de conexão via QR Code.

### Endpoint
```
POST /instance/connect
```

### Headers
```
Content-Type: application/json
apikey: TOKEN-DA-INSTANCIA
```

**⚠️ Atenção**: Use o `token` que você definiu ao criar a instância, NÃO a `GLOBAL_API_KEY`.

### Body
```json
{
  "webhookUrl": "https://seu-servidor.com/webhook",
  "subscribe": ["MESSAGE", "CONNECTION"]
}
```

Esta operação preserva as configurações existentes quando `subscribe` é omitido,
`null` ou uma lista vazia, ou quando os campos dos produtores são omitidos,
`null` ou strings vazias. Para desligar um produtor, envie `"disabled"` ou
`"false"`; para desativar o webhook, envie `"disabled"`. Uma instância sem eventos
configurados recebe o padrão `MESSAGE`.

Use os nomes de eventos da API, como `MESSAGE`, `CONNECTION` e `ALL`. Eventos
duplicados são removidos; uma lista não vazia sem nenhum evento válido retorna
400 e preserva as configurações. O corpo deve ser um objeto JSON; `null` é inválido.

### Resposta Sucesso (200)
```json
{
  "message": "success",
  "data": {
    "jid": "5511999999999@s.whatsapp.net",
    "webhookUrl": "https://seu-servidor.com/webhook",
    "eventString": "MESSAGE,CONNECTION"
  }
}
```

### Exemplo cURL
```bash
curl -X POST http://localhost:4000/instance/connect \
  -H "Content-Type: application/json" \
  -H "apikey: token-da-instancia-vendas" \
  -d '{
    "webhookUrl": "https://seu-servidor.com/webhook",
    "subscribe": ["MESSAGE"]
  }'
```

---

## Obter QR Code

Retorna o QR Code para escanear com WhatsApp.

### Endpoint
```
GET /instance/qr
```

### Headers
```
apikey: TOKEN-DA-INSTANCIA
```

**Importante**: A instância é identificada automaticamente pelo token no header.

### Resposta Sucesso (200)
```json
{
  "message": "success",
  "data": {
    "qrcode": "2@abcd1234...",
    "code": "data:image/png;base64,iVBORw0KGgoAAAANSUhEUg..."
  }
}
```

### Exemplo cURL
```bash
curl "http://localhost:4000/instance/qr" \
  -H "apikey: token-da-instancia-vendas"
```

---

## Conectar via Código de Pareamento

Conecta usando código de 8 dígitos em vez de QR Code.

### Endpoint
```
POST /instance/pair
```

### Headers
```
Content-Type: application/json
apikey: TOKEN-DA-INSTANCIA
```

### Body
```json
{
  "phone": "5511999999999"
}
```

### Resposta Sucesso (200)
```json
{
  "message": "success",
  "data": {
    "code": "12345678"
  }
}
```

### Como Usar
1. Faça a requisição com seu número
2. Receba o código de 8 dígitos
3. No WhatsApp: Aparelhos conectados > Conectar dispositivo > Conectar com número de telefone
4. Digite o código recebido

### Exemplo cURL
```bash
curl -X POST http://localhost:4000/instance/pair \
  -H "Content-Type: application/json" \
  -H "apikey: token-da-instancia-vendas" \
  -d '{"phone": "5511999999999"}'
```

---

## Status da Conexão

Verifica o status atual da instância.

### Endpoint
```
GET /instance/status
```

### Headers
```
apikey: TOKEN-DA-INSTANCIA
```

### Resposta Sucesso (200)
```json
{
  "message": "success",
  "data": {
    "connected": true,
    "loggedIn": true,
    "name": "Minha Empresa",
    "myJid": "5511999999999@s.whatsapp.net"
  }
}
```

### Status Possíveis
- `connected: false, loggedIn: false` - Instância criada mas não conectada
- `connected: true, loggedIn: false` - Conectando (aguardando QR Code)
- `connected: true, loggedIn: true` - Conectado e ativo
- `connected: false, loggedIn: true` - Sessão válida mas desconectado temporariamente

### Exemplo cURL
```bash
curl "http://localhost:4000/instance/status" \
  -H "apikey: token-da-instancia-vendas"
```

---

## Listar Todas as Instâncias

Retorna todas as instâncias cadastradas.

### Endpoint
```
GET /instance/all
```

### Headers
```
apikey: SUA-GLOBAL-API-KEY
```

**⚠️ Atenção**: Esta rota usa a `GLOBAL_API_KEY` (administrativa).

### Resposta Sucesso (200)
```json
{
  "message": "success",
  "data": [
    {
      "id": "abc123",
      "name": "vendas",
      "connected": true,
      "jid": "5511999999999@s.whatsapp.net"
    },
    {
      "id": "def456",
      "name": "suporte",
      "connected": false,
      "jid": null
    }
  ]
}
```

### Exemplo cURL
```bash
curl "http://localhost:4000/instance/all" \
  -H "apikey: SUA-GLOBAL-API-KEY"
```

---

## Informações da Instância

Obtém informações detalhadas de uma instância específica.

### Endpoint
```
GET /instance/info/:instanceId
```

### Headers
```
apikey: SUA-GLOBAL-API-KEY
```

**⚠️ Atenção**: Esta rota usa a `GLOBAL_API_KEY` (administrativa) e o ID da instância no path.

### Resposta Sucesso (200)
```json
{
  "message": "success",
  "data": {
    "id": "abc123",
    "name": "vendas",
    "token": "token-seguro",
    "webhook": "https://seu-servidor.com/webhook",
    "events": "messages.upsert,connection.update",
    "connected": true,
    "jid": "5511999999999@s.whatsapp.net",
    "createdAt": "2025-01-15T10:30:00Z"
  }
}
```

### Exemplo cURL
```bash
curl "http://localhost:4000/instance/info/vendas" \
  -H "apikey: SUA-GLOBAL-API-KEY"
```

---

## Desconectar Instância

Desconecta a instância mas mantém os dados.

### Endpoint
```
POST /instance/disconnect
```

### Headers
```
apikey: TOKEN-DA-INSTANCIA
```

### Body
```json
{}
```

**Nota**: O body pode ser vazio. A instância é identificada pelo header apikey.

### Resposta Sucesso (200)
```json
{
  "message": "success"
}
```

### Exemplo cURL
```bash
curl -X POST http://localhost:4000/instance/disconnect \
  -H "Content-Type: application/json" \
  -H "apikey: token-da-instancia-vendas" \
  -d '{}'
```

---

## Reconectar Instância

Reconecta uma instância desconectada.

### Endpoint
```
POST /instance/reconnect
```

### Headers
```
apikey: TOKEN-DA-INSTANCIA
```

### Body
```json
{}
```

### Resposta Sucesso (200)
```json
{
  "message": "success"
}
```

### Exemplo cURL
```bash
curl -X POST http://localhost:4000/instance/reconnect \
  -H "Content-Type: application/json" \
  -H "apikey: token-da-instancia-vendas" \
  -d '{}'
```

---

## Logout

Faz logout completo, removendo a sessão WhatsApp.

### Endpoint
```
DELETE /instance/logout
```

### Headers
```
apikey: TOKEN-DA-INSTANCIA
```

### Body
```json
{}
```

### Resposta Sucesso (200)
```json
{
  "message": "success"
}
```

**⚠️ Atenção**: Após logout, será necessário escanear QR Code novamente.

### Exemplo cURL
```bash
curl -X DELETE http://localhost:4000/instance/logout \
  -H "Content-Type: application/json" \
  -H "apikey: token-da-instancia-vendas" \
  -d '{}'
```

---

## Deletar Instância

Remove completamente a instância e todos os seus dados.

### Endpoint
```
DELETE /instance/delete/:instanceId
```

### Headers
```
apikey: SUA-GLOBAL-API-KEY
```

**⚠️ Atenção**: Esta rota usa a `GLOBAL_API_KEY` (administrativa). Esta ação é irreversível!

### Resposta Sucesso (200)
```json
{
  "message": "success"
}
```

### Exemplo cURL
```bash
curl -X DELETE "http://localhost:4000/instance/delete/vendas" \
  -H "apikey: SUA-GLOBAL-API-KEY"
```

---

## Configurar Proxy

Configura proxy HTTP para a instância.

### Endpoint
```
POST /instance/proxy/:instanceId
```

### Headers
```
Content-Type: application/json
apikey: SUA-GLOBAL-API-KEY
```

### Body
```json
{
  "host": "proxy.exemplo.com",
  "port": "8080",
  "username": "usuario",
  "password": "senha"
}
```

### Resposta Sucesso (200)
```json
{
  "message": "success",
  "data": {
    "host": "proxy.exemplo.com",
    "port": "8080",
    "hasAuth": true
  }
}
```

### Exemplo cURL
```bash
curl -X POST "http://localhost:4000/instance/proxy/vendas" \
  -H "Content-Type: application/json" \
  -H "apikey: SUA-GLOBAL-API-KEY" \
  -d '{
    "host": "proxy.exemplo.com",
    "port": "8080",
    "username": "usuario",
    "password": "senha"
  }'
```

---

## Remover Proxy

Remove configuração de proxy da instância.

### Endpoint
```
DELETE /instance/proxy/:instanceId
```

### Headers
```
apikey: SUA-GLOBAL-API-KEY
```

### Resposta Sucesso (200)
```json
{
  "message": "success"
}
```

### Exemplo cURL
```bash
curl -X DELETE "http://localhost:4000/instance/proxy/vendas" \
  -H "apikey: SUA-GLOBAL-API-KEY"
```

---

## Forçar Reconexão

Força reconexão com número específico (útil para resolver problemas).

### Endpoint
```
POST /instance/forcereconnect/:instanceId
```

### Headers
```
Content-Type: application/json
apikey: SUA-GLOBAL-API-KEY
```

### Body
```json
{
  "number": "5511999999999"
}
```

### Resposta Sucesso (200)
```json
{
  "message": "success"
}
```

### Exemplo cURL
```bash
curl -X POST "http://localhost:4000/instance/forcereconnect/vendas" \
  -H "Content-Type: application/json" \
  -H "apikey: SUA-GLOBAL-API-KEY" \
  -d '{"number": "5511999999999"}'
```

---

## Obter Logs

Obtém logs da instância.

### Endpoint
```
GET /instance/logs/:instanceId?start_date=DATA&end_date=DATA&level=NIVEL&limit=N
```

### Headers
```
apikey: SUA-GLOBAL-API-KEY
```

### Query Parameters
| Parâmetro | Tipo | Obrigatório | Descrição |
|-----------|------|-------------|-----------|
| start_date | string | ❌ Não | Data inicial (YYYY-MM-DD) |
| end_date | string | ❌ Não | Data final (YYYY-MM-DD) |
| level | string | ❌ Não | Nível do log (INFO, DEBUG, ERROR) |
| limit | integer | ❌ Não | Número de registros (padrão: 100) |

### Resposta Sucesso (200)
```json
[
  {
    "timestamp": "2025-01-15T10:30:00Z",
    "level": "INFO",
    "message": "Instance connected successfully",
    "instanceId": "vendas"
  }
]
```

### Exemplo cURL
```bash
curl "http://localhost:4000/instance/logs/vendas?start_date=2025-01-01&limit=50" \
  -H "apikey: SUA-GLOBAL-API-KEY"
```

---

## Configurações Avançadas

### Obter Configurações

```
GET /instance/:instanceId/advanced-settings
```

`instanceId` é o UUID retornado ao criar a instância e deve corresponder ao token
enviado no header `apikey`. Um ID de outra instância retorna 403; um ID inválido
retorna 400.

### Headers
```
apikey: TOKEN-DA-INSTANCIA
```

### Resposta (200)
```json
{
  "rejectCall": false,
  "msgRejectCall": "Não estou disponível para chamadas",
  "ignoreGroups": true,
  "alwaysOnline": false,
  "readMessages": false,
  "ignoreStatus": false
}
```

### Atualizar Configurações

```
PUT /instance/:instanceId/advanced-settings
```

Esta atualização é parcial: campos omitidos ou `null` mantêm os valores salvos.
Envie `false` para desativar uma flag ou `"msgRejectCall": ""` para apagar a
mensagem de rejeição. O corpo deve ser um objeto JSON; `{}` não altera os campos
e `null` retorna 400. O token deve pertencer ao UUID informado no path.

A resposta de sucesso contém `message` e `settings`, com todas as configurações
lidas do banco após a gravação. Falhas de gravação ou leitura retornam erro;
uma instância inexistente retorna 404. A gravação pode ter sido concluída quando
uma leitura posterior falha.

### Headers
```
Content-Type: application/json
apikey: TOKEN-DA-INSTANCIA
```

### Body
```json
{
  "rejectCall": true,
  "msgRejectCall": "Por favor, envie mensagem",
  "ignoreGroups": false,
  "alwaysOnline": true,
  "readMessages": true,
  "ignoreStatus": true
}
```

### Exemplo cURL
```bash
curl -X PUT "http://localhost:8081/instance/SEU-UUID/advanced-settings" \
  -H "Content-Type: application/json" \
  -H "apikey: TOKEN-DA-INSTANCIA" \
  -d '{
    "rejectCall": true,
    "alwaysOnline": true
  }'
```

---

## Fluxo Completo de Uso

### 1. Criar Instância (Administrativo)
```bash
curl -X POST http://localhost:4000/instance/create \
  -H "Content-Type: application/json" \
  -H "apikey: SUA-GLOBAL-API-KEY" \
  -d '{
    "name": "vendas",
    "token": "token-vendas-123"
  }'
```

### 2. Conectar (Usando Token da Instância)
```bash
curl -X POST http://localhost:4000/instance/connect \
  -H "Content-Type: application/json" \
  -H "apikey: token-vendas-123" \
  -d '{
    "webhookUrl": "https://meu-servidor.com/webhook",
    "subscribe": ["MESSAGE"]
  }'
```

### 3. Obter QR Code
```bash
curl "http://localhost:4000/instance/qr" \
  -H "apikey: token-vendas-123"
```

### 4. Escanear QR Code com WhatsApp

Abra o WhatsApp → Aparelhos conectados → Conectar um aparelho → Escanear QR Code

### 5. Verificar Status
```bash
curl "http://localhost:4000/instance/status" \
  -H "apikey: token-vendas-123"
```

---

## Boas Práticas

### 1. Gestão de Tokens

**GLOBAL_API_KEY**:
- Use para rotas administrativas (criar, deletar, listar instâncias)
- Mantenha em segredo (variável de ambiente)
- Nunca exponha em código frontend
- Rotacione periodicamente

**Token da Instância**:
- Defina ao criar a instância (`token` no body)
- Use para operações da instância específica (enviar mensagens, obter status)
- Um token único por instância
- Use UUIDs: `uuidgen` ou `python3 -c "import uuid; print(uuid.uuid4())"`

### 2. Nome de Instância
- Use nomes descritivos e únicos
- Apenas letras, números e hífen
- Exemplo: `vendas-empresa`, `suporte-01`

### 3. Webhooks
- Configure webhook para receber eventos em tempo real
- Use HTTPS em produção
- Valide os eventos recebidos
- Retorne status 200 rapidamente

### 4. Gerenciamento
- Monitore o status das instâncias regularmente
- Configure reconexão automática se necessário
- Mantenha logs para debug
- Use configurações avançadas conforme necessário

---

## Erros Comuns

### "not authorized"
**Causa**: API key incorreta ou ausente
**Solução**:
- Rotas administrativas: Use `GLOBAL_API_KEY`
- Rotas de operação: Use token da instância

### "instance not found"
**Causa**: Instância não existe ou token incorreto
**Solução**: Verifique se a instância foi criada e se está usando o token correto

### "name is required"
**Causa**: Campo obrigatório não enviado
**Solução**: Inclua todos os campos obrigatórios no body

### QR Code Expira
**Causa**: QR Code tem validade de ~40 segundos
**Solução**: Use `/instance/reconnect` para gerar novo QR

### "session already logged in"
**Causa**: Tentou obter QR Code de instância já conectada
**Solução**: Instância já está conectada, não precisa de QR Code

---

## Próximos Passos

- [API de Mensagens](./api-messages.md) - Enviar diferentes tipos de mensagens
- [API de Usuários](./api-user.md) - Gerenciar perfil e contatos
- [Sistema de Eventos](../recursos-avancados/events-system.md) - Webhooks e eventos
- [Configuração](../fundamentos/configuration.md) - Variáveis de ambiente

---

**Dica**: Use o Swagger em `http://localhost:4000/swagger/index.html` para testar os endpoints interativamente!
