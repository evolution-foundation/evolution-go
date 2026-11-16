# API de Usuários

Documentação completa dos endpoints para gerenciar perfil, contatos e privacidade.

## 📋 Índice

- [Informações do Usuário](#informações-do-usuário)
- [Verificar Usuário no WhatsApp](#verificar-usuário-no-whatsapp)
- [Avatar do Usuário](#avatar-do-usuário)
- [Listar Contatos](#listar-contatos)
- [Privacidade](#privacidade)
  - [Consultar Privacidade](#consultar-privacidade)
  - [Configurar Privacidade](#configurar-privacidade)
- [Bloqueio de Contatos](#bloqueio-de-contatos)
  - [Bloquear Contato](#bloquear-contato)
  - [Desbloquear Contato](#desbloquear-contato)
  - [Listar Bloqueados](#listar-bloqueados)
- [Perfil](#perfil)
  - [Alterar Foto de Perfil](#alterar-foto-de-perfil)
  - [Alterar Nome](#alterar-nome)
  - [Alterar Status/Recado](#alterar-statusrecado)

---

## Informações do Usuário

Obtém informações detalhadas de um ou mais usuários WhatsApp.

**Endpoint**: `POST /user/info`

**Headers**:
```
Content-Type: application/json
apikey: SUA-CHAVE-API
```

**Body**:
```json
{
  "number": ["5511999999999", "5511888888888"]
}
```

**Parâmetros**:

| Campo | Tipo | Obrigatório | Descrição |
|-------|------|-------------|-----------|
| `number` | array | ✅ Sim | Array de identificadores a consultar |

**Query canônica (ordem recomendada):**

1. `@lid` — quando conhecido (ex.: `123456789012345@lid`)
2. PN JID — `556699956041@s.whatsapp.net` (já canônico)
3. Dígitos — último recurso; números BR “sujos” (9º dígito extra) podem falhar ou retornar timeout rápido

Prefira o JID/`@lid` já gravado pela sessão. Para URL de foto confiável, use também `POST /user/avatar` (especialmente com `@lid`).

**Respostas de erro relevantes:**

| HTTP | Quando |
|------|--------|
| 400 | Corpo nulo/inválido, números ausentes ou identificador inválido |
| 429 | WhatsApp `rate-overlimit` (faça backoff; não trate como 500 genérico) |
| 504 | Timeout/cancelamento na preparação da sessão, resolução de LID ou query usync |
| 500 | Demais falhas |

A consulta possui um prazo total de 15s. A resolução de LIDs e o usync
compartilham um limite de 10s após a preparação da sessão. O enriquecimento
opcional de LID e foto compartilha até 5s, sempre respeitando o prazo restante
e o cancelamento da requisição. O início da sessão propaga o contexto às consultas
de banco; depois de iniciado, o cliente permanece sob responsabilidade do serviço.
Erros HTTP usam mensagens públicas, sem detalhes de banco ou credenciais.

**Resposta de Sucesso (200)**:
```json
{
  "message": "success",
  "data": {
    "Users": {
      "5511999999999@s.whatsapp.net": {
        "VerifiedName": {
          "Certificate": {...},
          "Details": {
            "Serial": 123,
            "Issuer": "WhatsApp",
            "VerifiedName": "Empresa Verificada LTDA"
          }
        },
        "Status": "Olá! Estou usando WhatsApp.",
        "PictureID": "abc123",
        "PictureURL": "https://pps.whatsapp.net/v/...",
        "Devices": ["5511999999999.0:1@s.whatsapp.net"],
        "LID": "lid_string"
      }
    }
  }
}
```

**Campos da Resposta**:
- `VerifiedName`: Nome verificado (empresas) ou null
- `Status`: Recado/status do usuário
- `PictureID`: ID da foto de perfil
- `PictureURL`: URL da foto de perfil (preview best-effort; vazio se indisponível, sem `PictureID`, sob rate-limit ou prazo esgotado). Para imagem completa use `POST /user/avatar`. A URL é retornada sem download da imagem. Falhas opcionais preservam todos os usuários e seus dados básicos na resposta 200.
- `Devices`: Lista de dispositivos conectados
- `LID`: Local ID (se disponível)

O LID retornado pelo usync é preservado diretamente. Quando ele estiver ausente,
a busca opcional no store usa o mesmo prazo do enriquecimento. Fotos são consultadas
em ordem estável de JID, sem enviar `ExistingID`; após um rate-limit, as consultas
opcionais restantes são interrompidas. Não há repetição automática de IQs.

**Exemplo cURL**:
```bash
curl -X POST http://localhost:${SERVER_PORT:-8080}/user/info \
  -H "Content-Type: application/json" \
  -H "apikey: SUA-CHAVE-API" \
  -d '{
    "number": ["5511999999999"]
  }'
```

---

## Verificar Usuário no WhatsApp

Verifica se um número existe no WhatsApp e retorna o JID correto para mensagens.

**Endpoint**: `POST /user/check`

**Body**:
```json
{
  "number": ["5511999999999", "11999999999", "+55 11 99999-9999"],
  "formatJid": false
}
```

**Parâmetros**:

| Campo | Tipo | Obrigatório | Descrição |
|-------|------|-------------|-----------|
| `number` | array | ✅ Sim | Array de números em qualquer formato |
| `formatJid` | bool | ❌ Não | Formatar número (padrão: false) |

**Nota Importante**: Por padrão, `formatJid=false` para verificação. O sistema tenta automaticamente ambos os formatos se o primeiro falhar.

**Resposta de Sucesso (200)**:
```json
{
  "message": "success",
  "data": {
    "Users": [
      {
        "Query": "5511999999999",
        "IsInWhatsapp": true,
        "JID": "5511999999999@s.whatsapp.net",
        "RemoteJID": "5511999999999@s.whatsapp.net",
        "LID": "lid_string",
        "VerifiedName": "Empresa LTDA"
      },
      {
        "Query": "5511888888888",
        "IsInWhatsapp": false,
        "JID": "",
        "RemoteJID": "5511888888888",
        "LID": null,
        "VerifiedName": ""
      }
    ]
  }
}
```

**Campos da Resposta**:
- `Query`: Número original consultado
- `IsInWhatsapp`: Se existe no WhatsApp
- `JID`: JID do usuário (vazio se não existe)
- `RemoteJID`: JID recomendado para envio de mensagens
- `LID`: Local ID
- `VerifiedName`: Nome verificado (empresas)

**Exemplo cURL**:
```bash
curl -X POST http://localhost:4000/user/check \
  -H "Content-Type: application/json" \
  -H "apikey: SUA-CHAVE-API" \
  -d '{
    "number": ["5511999999999", "11888888888"]
  }'
```

---

## Avatar do Usuário

Obtém a URL da foto de perfil de um usuário.

**Endpoint**: `POST /user/avatar`

**Body**:
```json
{
  "number": "5511999999999",
  "preview": false
}
```

**Parâmetros**:

| Campo | Tipo | Obrigatório | Descrição |
|-------|------|-------------|-----------|
| `number` | string | ✅ Sim | `@lid`, PN JID ou dígitos (mesma ordem canônica de `/user/info`) |
| `preview` | bool | ❌ Não | Se true, retorna preview (menor resolução) |

A requisição possui um prazo de 8s, incluindo banco no início da sessão,
espera pelo cliente autenticado, resolução de LID e query de foto. Prefira o JID
canônico já conhecido pela sessão. Cancelamento e timeout retornam **504**.

**Resposta de Sucesso (200)**:
```json
{
  "message": "success",
  "data": {
    "url": "https://pps.whatsapp.net/v/...",
    "id": "abc123",
    "type": "image",
    "direct_path": "/v/...",
    "hash": null
  }
}
```

**Respostas de erro**:

| HTTP | Exemplo |
|------|---------|
| 400 | Corpo nulo/inválido ou identificador ausente/inválido |
| 429 | WhatsApp `rate-overlimit` |
| 504 | Timeout/cancelamento na preparação da sessão, resolução de LID ou query de foto |
| 500 | Sem foto / foto oculta / demais falhas |

```json
{
  "error": "failed to query WhatsApp"
}
```

**Exemplo cURL**:
```bash
curl -X POST http://localhost:${SERVER_PORT:-8080}/user/avatar \
  -H "Content-Type: application/json" \
  -H "apikey: SUA-CHAVE-API" \
  -d '{
    "number": "5511999999999",
    "preview": true
  }'
```

---

## Listar Contatos

Obtém todos os contatos salvos na conta WhatsApp.

**Endpoint**: `GET /user/contacts`

**Headers**:
```
apikey: SUA-CHAVE-API
```

**Resposta de Sucesso (200)**:
```json
{
  "message": "success",
  "data": [
    {
      "Jid": "5511999999999@s.whatsapp.net",
      "Found": true,
      "FirstName": "João",
      "FullName": "João Silva",
      "PushName": "João",
      "BusinessName": ""
    },
    {
      "Jid": "5511888888888@s.whatsapp.net",
      "Found": true,
      "FirstName": "Maria",
      "FullName": "Maria Santos",
      "PushName": "Maria",
      "BusinessName": "Loja da Maria"
    }
  ]
}
```

**Campos da Resposta**:
- `Jid`: JID do contato
- `Found`: Se foi encontrado no WhatsApp
- `FirstName`: Primeiro nome
- `FullName`: Nome completo
- `PushName`: Nome exibido no WhatsApp
- `BusinessName`: Nome da empresa (se for conta comercial)

**Exemplo cURL**:
```bash
curl -X GET http://localhost:4000/user/contacts \
  -H "apikey: SUA-CHAVE-API"
```

---

## Privacidade

### Consultar Privacidade

Obtém as configurações atuais de privacidade da conta.

**Endpoint**: `GET /user/privacy`

**Headers**:
```
apikey: SUA-CHAVE-API
```

**Resposta de Sucesso (200)**:
```json
{
  "message": "success",
  "data": {
    "GroupAdd": "all",
    "LastSeen": "contacts",
    "Status": "contacts",
    "Profile": "all",
    "ReadReceipts": "all",
    "CallAdd": "all",
    "Online": "all"
  }
}
```

**Valores Possíveis**:
- `all` - Todos
- `contacts` - Apenas contatos
- `contact_blacklist` - Meus contatos exceto...
- `none` - Ninguém
- `match_last_seen` - Mesmo de "Visto por último"

**Exemplo cURL**:
```bash
curl -X GET http://localhost:4000/user/privacy \
  -H "apikey: SUA-CHAVE-API"
```

---

### Configurar Privacidade

Define as configurações de privacidade da conta.

**Endpoint**: `POST /user/privacy`

**Body**:
```json
{
  "groupAdd": "contacts",
  "lastSeen": "contacts",
  "status": "contacts",
  "profile": "all",
  "readReceipts": "all",
  "callAdd": "all",
  "online": "all"
}
```

**Parâmetros**:

| Campo | Tipo | Obrigatório | Descrição |
|-------|------|-------------|-----------|
| `groupAdd` | string | ✅ Sim | Quem pode me adicionar em grupos |
| `lastSeen` | string | ✅ Sim | Quem vê meu "visto por último" |
| `status` | string | ✅ Sim | Quem vê meu recado/status |
| `profile` | string | ✅ Sim | Quem vê minha foto de perfil |
| `readReceipts` | string | ✅ Sim | Confirmações de leitura |
| `callAdd` | string | ✅ Sim | Quem pode me ligar |
| `online` | string | ✅ Sim | Quem vê quando estou online |

**Valores Permitidos**: `all`, `contacts`, `contact_blacklist`, `none`, `match_last_seen`

**Resposta de Sucesso (200)**:
```json
{
  "message": "success",
  "data": {
    "GroupAdd": "contacts",
    "LastSeen": "contacts",
    "Status": "contacts",
    "Profile": "all",
    "ReadReceipts": "all",
    "CallAdd": "all",
    "Online": "all"
  }
}
```

**Exemplo cURL**:
```bash
curl -X POST http://localhost:4000/user/privacy \
  -H "Content-Type: application/json" \
  -H "apikey: SUA-CHAVE-API" \
  -d '{
    "groupAdd": "contacts",
    "lastSeen": "contacts",
    "status": "contacts",
    "profile": "all",
    "readReceipts": "all",
    "callAdd": "all",
    "online": "all"
  }'
```

---

## Bloqueio de Contatos

### Bloquear Contato

Bloqueia um contato no WhatsApp.

**Endpoint**: `POST /user/block`

**Body**:
```json
{
  "number": "5511999999999"
}
```

**Parâmetros**:

| Campo | Tipo | Obrigatório | Descrição |
|-------|------|-------------|-----------|
| `number` | string | ✅ Sim | Número do contato a bloquear |

**Resposta de Sucesso (200)**:
```json
{
  "message": "success",
  "data": {
    "DHash": "abc123",
    "PrevDHash": "def456",
    "Modifications": [
      {
        "JID": "5511999999999@s.whatsapp.net",
        "Action": "block"
      }
    ]
  }
}
```

**Exemplo cURL**:
```bash
curl -X POST http://localhost:4000/user/block \
  -H "Content-Type: application/json" \
  -H "apikey: SUA-CHAVE-API" \
  -d '{
    "number": "5511999999999"
  }'
```

---

### Desbloquear Contato

Desbloqueia um contato previamente bloqueado.

**Endpoint**: `POST /user/unblock`

**Body**:
```json
{
  "number": "5511999999999"
}
```

**Parâmetros**:

| Campo | Tipo | Obrigatório | Descrição |
|-------|------|-------------|-----------|
| `number` | string | ✅ Sim | Número do contato a desbloquear |

**Resposta de Sucesso (200)**:
```json
{
  "message": "success",
  "data": {
    "DHash": "abc123",
    "PrevDHash": "def456",
    "Modifications": [
      {
        "JID": "5511999999999@s.whatsapp.net",
        "Action": "unblock"
      }
    ]
  }
}
```

**Exemplo cURL**:
```bash
curl -X POST http://localhost:4000/user/unblock \
  -H "Content-Type: application/json" \
  -H "apikey: SUA-CHAVE-API" \
  -d '{
    "number": "5511999999999"
  }'
```

---

### Listar Bloqueados

Obtém a lista de todos os contatos bloqueados.

**Endpoint**: `GET /user/blocklist`

**Headers**:
```
apikey: SUA-CHAVE-API
```

**Resposta de Sucesso (200)**:
```json
{
  "message": "success",
  "data": {
    "DHash": "abc123",
    "Modifications": [
      {
        "JID": "5511999999999@s.whatsapp.net",
        "Action": "block"
      },
      {
        "JID": "5511888888888@s.whatsapp.net",
        "Action": "block"
      }
    ]
  }
}
```

**Exemplo cURL**:
```bash
curl -X GET http://localhost:4000/user/blocklist \
  -H "apikey: SUA-CHAVE-API"
```

---

## Perfil

### Alterar Foto de Perfil

Define a foto de perfil da conta WhatsApp.

**Endpoint**: `POST /user/profilePicture`

**Body**:
```json
{
  "image": "https://exemplo.com/foto.jpg"
}
```

**Parâmetros**:

| Campo | Tipo | Obrigatório | Descrição |
|-------|------|-------------|-----------|
| `image` | string | ✅ Sim | URL da imagem |

**Nota**: A imagem deve ser acessível via HTTP/HTTPS. Formatos aceitos: JPG, PNG.

**Resposta de Sucesso (200)**:
```json
{
  "message": "success",
  "data": {
    "image": "https://exemplo.com/foto.jpg"
  }
}
```

**Exemplo cURL**:
```bash
curl -X POST http://localhost:4000/user/profilePicture \
  -H "Content-Type: application/json" \
  -H "apikey: SUA-CHAVE-API" \
  -d '{
    "image": "https://exemplo.com/minha-foto.jpg"
  }'
```

---

### Alterar Nome

Define o nome de exibição da conta WhatsApp.

**Endpoint**: `POST /user/profileName`

**Body**:
```json
{
  "name": "João Silva"
}
```

**Parâmetros**:

| Campo | Tipo | Obrigatório | Descrição |
|-------|------|-------------|-----------|
| `name` | string | ✅ Sim | Novo nome de exibição |

**Resposta de Sucesso (200)**:
```json
{
  "message": "success",
  "data": {
    "name": "João Silva"
  }
}
```

**Exemplo cURL**:
```bash
curl -X POST http://localhost:4000/user/profileName \
  -H "Content-Type: application/json" \
  -H "apikey: SUA-CHAVE-API" \
  -d '{
    "name": "Meu Novo Nome"
  }'
```

---

### Alterar Status/Recado

Define o recado (status) da conta WhatsApp.

**Endpoint**: `POST /user/profileStatus`

**Body**:
```json
{
  "status": "Disponível para atendimento 24h!"
}
```

**Parâmetros**:

| Campo | Tipo | Obrigatório | Descrição |
|-------|------|-------------|-----------|
| `status` | string | ✅ Sim | Novo recado/status |

**Resposta de Sucesso (200)**:
```json
{
  "message": "success",
  "data": {
    "status": "Disponível para atendimento 24h!"
  }
}
```

**Exemplo cURL**:
```bash
curl -X POST http://localhost:4000/user/profileStatus \
  -H "Content-Type: application/json" \
  -H "apikey: SUA-CHAVE-API" \
  -d '{
    "status": "Olá! Como posso ajudar?"
  }'
```

---

## Casos de Uso Comuns

### Validar Números Antes de Enviar

Sempre verifique se o número existe antes de tentar enviar mensagem:

```bash
# 1. Verificar número
curl -X POST http://localhost:4000/user/check \
  -H "Content-Type: application/json" \
  -H "apikey: SUA-CHAVE-API" \
  -d '{
    "number": ["5511999999999"]
  }'

# 2. Se IsInWhatsapp=true, use RemoteJID para enviar
curl -X POST http://localhost:4000/send/text \
  -H "Content-Type: application/json" \
  -H "apikey: SUA-CHAVE-API" \
  -d '{
    "number": "5511999999999@s.whatsapp.net",
    "text": "Olá!",
    "formatJid": false
  }'
```

### Configurar Privacidade Máxima

```bash
curl -X POST http://localhost:4000/user/privacy \
  -H "Content-Type: application/json" \
  -H "apikey: SUA-CHAVE-API" \
  -d '{
    "groupAdd": "contacts",
    "lastSeen": "none",
    "status": "contacts",
    "profile": "contacts",
    "readReceipts": "all",
    "callAdd": "contacts",
    "online": "none"
  }'
```

### Personalizar Perfil Completo

```bash
# 1. Foto de perfil
curl -X POST http://localhost:4000/user/profilePicture \
  -H "Content-Type: application/json" \
  -H "apikey: SUA-CHAVE-API" \
  -d '{"image": "https://exemplo.com/logo.jpg"}'

# 2. Nome
curl -X POST http://localhost:4000/user/profileName \
  -H "Content-Type: application/json" \
  -H "apikey: SUA-CHAVE-API" \
  -d '{"name": "Empresa LTDA"}'

# 3. Status
curl -X POST http://localhost:4000/user/profileStatus \
  -H "Content-Type: application/json" \
  -H "apikey: SUA-CHAVE-API" \
  -d '{"status": "Atendimento 24h - (11) 99999-9999"}'
```

---

## Códigos de Erro Comuns

| Código | Erro | Solução |
|--------|------|---------|
| 400 | `phone number is required` | Forneça o campo `number` |
| 400 | `image is required` | Forneça uma URL de imagem válida |
| 500 | `instance not found` | Instância não conectada |
| 500 | `no profile picture found` | Usuário não tem foto de perfil |
| 500 | `invalid phone number` | Formato de número inválido |

---

## Boas Práticas

### 1. Cache de Verificações
Evite verificar o mesmo número múltiplas vezes. Implemente um sistema de cache na sua aplicação:
- Armazene o resultado da verificação por algumas horas
- Antes de chamar a API, verifique se já tem o resultado em cache
- Isso reduz requisições desnecessárias e melhora a performance

### 2. Privacidade Responsável
Configure privacidade adequada para contas comerciais:
- `readReceipts: all` - Envie confirmações de leitura
- `online: contacts` - Evite mostrar online para todos
- `groupAdd: contacts` - Evite spam de grupos

### 3. Validação de Imagens
Ao alterar foto de perfil, certifique-se que a imagem:
- Está acessível publicamente (HTTP/HTTPS)
- É JPG ou PNG
- Tem tamanho adequado (recomendado: 640x640px)

---

## Próximos Passos

- [API de Mensagens](./api-messages.md) - Enviar e gerenciar mensagens
- [API de Grupos](./api-groups.md) - Gerenciar grupos WhatsApp
- [API de Chats](./api-chats.md) - Gerenciar conversas
- [Visão Geral da API](./api-overview.md)

---

**Documentação gerada para Evolution GO v1.0**
