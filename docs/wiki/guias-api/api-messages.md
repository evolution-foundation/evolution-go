# API de Mensagens

Documentação completa dos endpoints para enviar e gerenciar mensagens WhatsApp.

## 📋 Índice

### Enviar Mensagens
- [Enviar Texto](#enviar-texto)
- [Enviar Link com Preview](#enviar-link-com-preview)
- [Enviar Mídia](#enviar-mídia)
- [Enviar Enquete (Poll)](#enviar-enquete)
- [Enviar Sticker](#enviar-sticker)
- [Enviar Localização](#enviar-localização)
- [Enviar Contato](#enviar-contato)
- [Enviar Botões](api-interactive.md#enviar-botões) ➡️ **Ver [API de Mensagens Interativas](api-interactive.md)**
- [Enviar Lista](api-interactive.md#enviar-lista) ➡️ **Ver [API de Mensagens Interativas](api-interactive.md)**
- [Enviar Carrossel](api-interactive.md#enviar-carrossel) ➡️ **Ver [API de Mensagens Interativas](api-interactive.md)**

### Gerenciar Mensagens
- [Reagir a Mensagem](#reagir-a-mensagem)
- [Marcar como Lida](#marcar-como-lida)
- [Editar Mensagem](#editar-mensagem)
- [Deletar Mensagem](#deletar-mensagem)
- [Encaminhar Mensagem](#encaminhar-mensagem)
- [Presença no Chat](#presença-no-chat)
- [Download de Mídia](#download-de-mídia)
- [Status da Mensagem](#status-da-mensagem)

---

## Enviar Mensagens

### Enviar Texto

Envia uma mensagem de texto simples.

**Endpoint**: `POST /send/text`

**Headers**:
```
Content-Type: application/json
apikey: SUA-CHAVE-API
```

**Body**:
```json
{
  "number": "5511999999999",
  "text": "Olá! Como posso ajudar?",
  "id": "msg-custom-123",
  "delay": 1000,
  "mentionedJid": "5511888888888@s.whatsapp.net",
  "mentionAll": false,
  "formatJid": true,
  "quoted": {
    "messageId": "BAE5...",
    "participant": "5511999999999@s.whatsapp.net"
  }
}
```

**Parâmetros**:

| Campo | Tipo | Obrigatório | Descrição |
|-------|------|-------------|-----------|
| `number` | string | ✅ Sim | Número do destinatário (formato: DDI + DDD + número) |
| `text` | string | ✅ Sim | Texto da mensagem |
| `id` | string | ❌ Não | ID customizado (se vazio, será gerado automaticamente) |
| `delay` | int32 | ❌ Não | Delay em milissegundos antes de enviar |
| `mentionedJid` | string | ❌ Não | JID do usuário a mencionar |
| `mentionAll` | bool | ❌ Não | Mencionar todos os participantes (apenas grupos) |
| `formatJid` | bool | ❌ Não | Formatar número automaticamente (padrão: true) |
| `quoted` | object | ❌ Não | Mensagem a ser citada |

**Resposta de Sucesso (200)**:
```json
{
  "message": "success",
  "data": {
    "Info": {
      "ID": "3EB0C5A277F7F9B6C599",
      "ServerID": 12345,
      "Timestamp": "2025-11-11T10:30:00Z",
      "Type": "ExtendedTextMessage"
    }
  }
}
```

**Resposta de Erro (400)**:
```json
{
  "error": "phone number is required"
}
```

**Exemplo cURL**:
```bash
curl -X POST http://localhost:4000/send/text \
  -H "Content-Type: application/json" \
  -H "apikey: SUA-CHAVE-API" \
  -d '{
    "number": "5511999999999",
    "text": "Olá! Esta é uma mensagem de teste."
  }'
```

---

### Enviar Link com Preview

Envia uma mensagem com preview de link (título, descrição, imagem).

**Endpoint**: `POST /send/link`

**Body**:
```json
{
  "number": "5511999999999",
  "text": "Confira este artigo: https://example.com/artigo",
  "title": "Título do Link",
  "url": "https://example.com/artigo",
  "description": "Descrição do conteúdo",
  "imgUrl": "https://example.com/imagem.jpg"
}
```

**Parâmetros**:

| Campo | Tipo | Obrigatório | Descrição |
|-------|------|-------------|-----------|
| `number` | string | ✅ Sim | Número do destinatário |
| `text` | string | ✅ Sim | Texto com URL |
| `title` | string | ❌ Não | Título do preview (extraído automaticamente se vazio) |
| `url` | string | ❌ Não | URL do link |
| `description` | string | ❌ Não | Descrição (extraída automaticamente se vazia) |
| `imgUrl` | string | ❌ Não | URL da imagem de preview |

**Nota**: Se `title`, `description` ou `imgUrl` não forem fornecidos, o sistema tentará extrair automaticamente os metadados Open Graph da URL.

**Resposta de Sucesso (200)**:
```json
{
  "message": "success",
  "data": {
    "Info": {
      "ID": "3EB0C5A277F7F9B6C599",
      "Type": "ExtendedTextMessage"
    }
  }
}
```

**Exemplo cURL**:
```bash
curl -X POST http://localhost:4000/send/link \
  -H "Content-Type: application/json" \
  -H "apikey: SUA-CHAVE-API" \
  -d '{
    "number": "5511999999999",
    "text": "Veja esta notícia: https://g1.globo.com/tecnologia"
  }'
```

---

### Enviar Mídia

Envia imagem, vídeo, áudio ou documento. Suporta envio via **URL** ou **arquivo local** (multipart/form-data).

**Endpoint**: `POST /send/media`

#### Opção 1: Enviar por URL

**Body (JSON)**:
```json
{
  "number": "5511999999999",
  "url": "https://example.com/imagem.jpg",
  "type": "image",
  "caption": "Confira esta imagem!",
  "filename": "foto.jpg"
}
```

#### Opção 2: Enviar Arquivo (multipart/form-data)

**Body (form-data)**:
```
number: 5511999999999
type: image
caption: Confira esta imagem!
filename: foto.jpg
file: [arquivo binário]
delay: 0
```

**Parâmetros**:

| Campo | Tipo | Obrigatório | Descrição |
|-------|------|-------------|-----------|
| `number` | string | ✅ Sim | Número do destinatário |
| `type` | string | ✅ Sim | Tipo: `image`, `video`, `audio`, `document` |
| `url` | string | ✅ Sim (URL) | URL da mídia (se não enviar arquivo) |
| `file` | binary | ✅ Sim (arquivo) | Arquivo binário (se não enviar URL) |
| `caption` | string | ❌ Não | Legenda da mídia |
| `filename` | string | ❌ Não | Nome do arquivo |

**Tipos de Mídia Aceitos**:

| Tipo | Formatos Aceitos | Observações |
|------|------------------|-------------|
| `image` | JPG, PNG, WebP | WebP convertido para JPEG |
| `video` | MP4 | Apenas MP4 |
| `audio` | Qualquer | Convertido para Opus (PTT) automaticamente |
| `document` | Qualquer | Qualquer tipo de arquivo |

**Áudio**: O sistema converte automaticamente qualquer formato de áudio para **Opus** (formato PTT do WhatsApp). Pode usar conversor local (ffmpeg) ou API externa (configurável via `API_AUDIO_CONVERTER`).

**Resposta de Sucesso (200)**:
```json
{
  "message": "success",
  "data": {
    "Info": {
      "ID": "3EB0C5A277F7F9B6C599",
      "Type": "ImageMessage"
    }
  }
}
```

**Resposta de Erro (400)**:
```json
{
  "error": "Invalid file format: 'image/gif'. Only 'image/jpeg', 'image/png' and 'image/webp' are accepted"
}
```

**Exemplo cURL (URL)**:
```bash
curl -X POST http://localhost:4000/send/media \
  -H "Content-Type: application/json" \
  -H "apikey: SUA-CHAVE-API" \
  -d '{
    "number": "5511999999999",
    "url": "https://exemplo.com/produto.jpg",
    "type": "image",
    "caption": "Novo produto disponível!"
  }'
```

**Exemplo cURL (Arquivo)**:
```bash
curl -X POST http://localhost:4000/send/media \
  -H "apikey: SUA-CHAVE-API" \
  -F "number=5511999999999" \
  -F "type=image" \
  -F "caption=Foto enviada" \
  -F "file=@/caminho/para/imagem.jpg"
```

---

### Enviar Enquete

Cria uma enquete (poll) com múltiplas opções.

**Endpoint**: `POST /send/poll`

**Body**:
```json
{
  "number": "5511999999999",
  "question": "Qual seu horário preferido?",
  "maxAnswer": 1,
  "options": [
    "Manhã (8h-12h)",
    "Tarde (13h-18h)",
    "Noite (19h-22h)"
  ]
}
```

**Parâmetros**:

| Campo | Tipo | Obrigatório | Descrição |
|-------|------|-------------|-----------|
| `number` | string | ✅ Sim | Número do destinatário |
| `question` | string | ✅ Sim | Pergunta da enquete |
| `options` | array | ✅ Sim | Opções (mínimo 2) |
| `maxAnswer` | int | ❌ Não | Número máximo de respostas permitidas |

**Resposta de Sucesso (200)**:
```json
{
  "message": "success",
  "data": {
    "Info": {
      "ID": "3EB0C5A277F7F9B6C599",
      "Type": "PollCreationMessage"
    }
  }
}
```

**Resposta de Erro (400)**:
```json
{
  "error": "minimum 2 options are required"
}
```

**Exemplo cURL**:
```bash
curl -X POST http://localhost:4000/send/poll \
  -H "Content-Type: application/json" \
  -H "apikey: SUA-CHAVE-API" \
  -d '{
    "number": "5511999999999",
    "question": "Qual plano você prefere?",
    "maxAnswer": 1,
    "options": ["Básico", "Intermediário", "Premium"]
  }'
```

---

### Enviar Sticker

Envia um sticker (figurinha) via URL.

**Endpoint**: `POST /send/sticker`

**Body**:
```json
{
  "number": "5511999999999",
  "sticker": "https://example.com/sticker.webp"
}
```

**Parâmetros**:

| Campo | Tipo | Obrigatório | Descrição |
|-------|------|-------------|-----------|
| `number` | string | ✅ Sim | Número do destinatário |
| `sticker` | string | ✅ Sim | URL da imagem (convertida para WebP automaticamente) |

**Nota**: O sistema converte automaticamente a imagem para o formato WebP (formato de sticker do WhatsApp).

**Resposta de Sucesso (200)**:
```json
{
  "message": "success",
  "data": {
    "Info": {
      "ID": "3EB0C5A277F7F9B6C599",
      "Type": "StickerMessage"
    }
  }
}
```

**Exemplo cURL**:
```bash
curl -X POST http://localhost:4000/send/sticker \
  -H "Content-Type: application/json" \
  -H "apikey: SUA-CHAVE-API" \
  -d '{
    "number": "5511999999999",
    "sticker": "https://exemplo.com/figurinha.png"
  }'
```

---

### Enviar Localização

Envia uma localização geográfica.

**Endpoint**: `POST /send/location`

**Body**:
```json
{
  "number": "5511999999999",
  "name": "Escritório Central",
  "address": "Av. Paulista, 1000 - São Paulo, SP",
  "latitude": -23.5505199,
  "longitude": -46.6333094
}
```

**Parâmetros**:

| Campo | Tipo | Obrigatório | Descrição |
|-------|------|-------------|-----------|
| `number` | string | ✅ Sim | Número do destinatário |
| `latitude` | float64 | ✅ Sim | Latitude da localização |
| `longitude` | float64 | ✅ Sim | Longitude da localização |
| `name` | string | ✅ Sim | Nome do local |
| `address` | string | ✅ Sim | Endereço do local |

**Resposta de Sucesso (200)**:
```json
{
  "message": "success",
  "data": {
    "Info": {
      "ID": "3EB0C5A277F7F9B6C599",
      "Type": "LocationMessage"
    }
  }
}
```

**Exemplo cURL**:
```bash
curl -X POST http://localhost:4000/send/location \
  -H "Content-Type: application/json" \
  -H "apikey: SUA-CHAVE-API" \
  -d '{
    "number": "5511999999999",
    "name": "Teatro Municipal",
    "address": "Praça Ramos de Azevedo, São Paulo",
    "latitude": -23.5454614,
    "longitude": -46.6369813
  }'
```

---

### Enviar Contato

Envia um cartão de contato (VCard).

**Endpoint**: `POST /send/contact`

**Body**:
```json
{
  "number": "5511999999999",
  "vcard": {
    "fullName": "João Silva",
    "phone": "5511888888888",
    "organization": "Empresa LTDA"
  }
}
```

**Parâmetros**:

| Campo | Tipo | Obrigatório | Descrição |
|-------|------|-------------|-----------|
| `number` | string | ✅ Sim | Número do destinatário |
| `vcard.fullName` | string | ✅ Sim | Nome completo do contato |
| `vcard.phone` | string | ✅ Sim | Telefone do contato |
| `vcard.organization` | string | ❌ Não | Empresa/organização |

**Resposta de Sucesso (200)**:
```json
{
  "message": "success",
  "data": {
    "Info": {
      "ID": "3EB0C5A277F7F9B6C599",
      "Type": "ContactMessage"
    }
  }
}
```

**Exemplo cURL**:
```bash
curl -X POST http://localhost:4000/send/contact \
  -H "Content-Type: application/json" \
  -H "apikey: SUA-CHAVE-API" \
  -d '{
    "number": "5511999999999",
    "vcard": {
      "fullName": "Maria Santos",
      "phone": "5511777777777",
      "organization": "Vendas LTDA"
    }
  }'
```

---

### Enviar Botões

> ⚠️ **ENDPOINT DEPRECIADO**
> 
> Este endpoint **não funciona mais**. O WhatsApp descontinuou o suporte a botões interativos para contas que não são Business API oficial.
> 
> **Alternativas**:
> - Use **Enquetes (Polls)** para coleta de respostas
> - Use **mensagens de texto** com instruções
> - Para soluções avançadas, migre para WhatsApp Business API oficial

~~Envia mensagem com botões interativos. Suporta diferentes tipos de botões.~~

**Endpoint**: ~~`POST /send/button`~~

**Body**:
```json
{
  "number": "5511999999999",
  "title": "Escolha uma opção",
  "description": "Selecione o que deseja fazer",
  "footer": "Powered by Evolution GO",
  "buttons": [
    {
      "type": "reply",
      "displayText": "Ver Produtos",
      "id": "btn_produtos"
    },
    {
      "type": "url",
      "displayText": "Site Oficial",
      "url": "https://exemplo.com"
    },
    {
      "type": "call",
      "displayText": "Ligar",
      "phoneNumber": "5511999999999"
    }
  ]
}
```

**Parâmetros**:

| Campo | Tipo | Obrigatório | Descrição |
|-------|------|-------------|-----------|
| `number` | string | ✅ Sim | Número do destinatário |
| `title` | string | ✅ Sim | Título da mensagem |
| `description` | string | ✅ Sim | Descrição/corpo da mensagem |
| `footer` | string | ✅ Sim | Rodapé da mensagem |
| `buttons` | array | ✅ Sim | Array de botões (máx 3 para tipo reply) |

**Tipos de Botões**:

| Tipo | Campos Necessários | Descrição | Limitações |
|------|-------------------|-----------|------------|
| `reply` | `displayText`, `id` | Botão de resposta rápida | Máx 3, não pode misturar com outros tipos |
| `copy` | `displayText`, `copyCode` | Copiar texto | - |
| `url` | `displayText`, `url` | Abrir URL | - |
| `call` | `displayText`, `phoneNumber` | Ligar para número | - |
| `pix` | `name`, `key`, `keyType`, `currency` | Pagamento PIX (Brasil) | Não pode combinar com outros |

**Botão PIX**:
```json
{
  "type": "pix",
  "name": "Loja Exemplo",
  "key": "exemplo@pix.com",
  "keyType": "email",
  "currency": "BRL"
}
```

Tipos de chave PIX: `phone`, `email`, `cpf`, `cnpj`, `random` (EVP).

**Resposta de Sucesso (200)**:
```json
{
  "message": "success",
  "data": {
    "Info": {
      "ID": "3EB0C5A277F7F9B6C599",
      "Type": "ButtonMessage"
    }
  }
}
```

**Resposta de Erro (400)**:
```json
{
  "error": "máximo de 3 botões do tipo 'reply' permitidos"
}
```

**Exemplo cURL**:
```bash
curl -X POST http://localhost:4000/send/button \
  -H "Content-Type: application/json" \
  -H "apikey: SUA-CHAVE-API" \
  -d '{
    "number": "5511999999999",
    "title": "Atendimento",
    "description": "Como podemos ajudar?",
    "footer": "Suporte 24h",
    "buttons": [
      {"type": "reply", "displayText": "Falar com Vendas", "id": "vendas"},
      {"type": "reply", "displayText": "Suporte Técnico", "id": "suporte"}
    ]
  }'
```

---

### Enviar Lista

> ⚠️ **ENDPOINT DEPRECIADO**
> 
> Este endpoint **não funciona mais**. O WhatsApp descontinuou o suporte a listas interativas para contas que não são Business API oficial.
> 
> **Alternativas**:
> - Use **Enquetes (Polls)** para seleção de opções
> - Use **mensagens de texto** com numeração
> - Para soluções avançadas, migre para WhatsApp Business API oficial

~~Envia mensagem com menu de lista interativo.~~

**Endpoint**: ~~`POST /send/list`~~

**Body**:
```json
{
  "number": "5511999999999",
  "title": "Nossos Serviços",
  "description": "Selecione um serviço",
  "buttonText": "Ver Opções",
  "footerText": "Atendimento 24h",
  "sections": [
    {
      "title": "Planos",
      "rows": [
        {
          "title": "Plano Básico",
          "description": "R$ 29,90/mês",
          "rowId": "plano_basico"
        },
        {
          "title": "Plano Premium",
          "description": "R$ 59,90/mês",
          "rowId": "plano_premium"
        }
      ]
    },
    {
      "title": "Suporte",
      "rows": [
        {
          "title": "Falar com Atendente",
          "description": "Chat ao vivo",
          "rowId": "atendente"
        }
      ]
    }
  ]
}
```

**Parâmetros**:

| Campo | Tipo | Obrigatório | Descrição |
|-------|------|-------------|-----------|
| `number` | string | ✅ Sim | Número do destinatário |
| `title` | string | ✅ Sim | Título da mensagem |
| `description` | string | ✅ Sim | Descrição/corpo |
| `buttonText` | string | ✅ Sim | Texto do botão que abre a lista |
| `footerText` | string | ✅ Sim | Rodapé da mensagem |
| `sections` | array | ✅ Sim | Seções da lista |

**Estrutura de Section**:

| Campo | Tipo | Obrigatório | Descrição |
|-------|------|-------------|-----------|
| `title` | string | ✅ Sim | Título da seção |
| `rows` | array | ✅ Sim | Linhas da seção |

**Estrutura de Row**:

| Campo | Tipo | Obrigatório | Descrição |
|-------|------|-------------|-----------|
| `title` | string | ✅ Sim | Título da opção |
| `description` | string | ❌ Não | Descrição da opção |
| `rowId` | string | ✅ Sim | ID único da opção |

**Resposta de Sucesso (200)**:
```json
{
  "message": "success",
  "data": {
    "Info": {
      "ID": "3EB0C5A277F7F9B6C599",
      "Type": "ListMessage"
    }
  }
}
```

**Exemplo cURL**:
```bash
curl -X POST http://localhost:4000/send/list \
  -H "Content-Type: application/json" \
  -H "apikey: SUA-CHAVE-API" \
  -d '{
    "number": "5511999999999",
    "title": "Menu Principal",
    "description": "Escolha uma categoria",
    "buttonText": "Ver Menu",
    "footerText": "Delivery 24h",
    "sections": [
      {
        "title": "Pratos",
        "rows": [
          {"title": "Pizza", "description": "A partir de R$ 35", "rowId": "pizza"},
          {"title": "Hambúrguer", "description": "A partir de R$ 25", "rowId": "burger"}
        ]
      }
    ]
  }'
```

---

## Gerenciar Mensagens

### Reagir a Mensagem

Adiciona ou remove uma reação (emoji) em uma mensagem.

**Endpoint**: `POST /message/react`

**Body**:
```json
{
  "number": "5511999999999",
  "reaction": "👍",
  "id": "3EB0C5A277F7F9B6C599",
  "fromMe": false,
  "participant": "5511888888888@s.whatsapp.net"
}
```

**Parâmetros**:

| Campo | Tipo | Obrigatório | Descrição |
|-------|------|-------------|-----------|
| `number` | string | ✅ Sim | Número do chat (individual ou grupo) |
| `reaction` | string | ✅ Sim | Emoji da reação (ou "remove" para remover) |
| `id` | string | ✅ Sim | ID da mensagem a reagir |
| `fromMe` | bool | ✅ Sim | Se a mensagem foi enviada por você (true/false) |
| `participant` | string | ❌ Não | JID do autor (obrigatório em grupos quando fromMe=false) |

**Nota**: Para remover uma reação, use `"reaction": "remove"`.

**Resposta de Sucesso (200)**:
```json
{
  "message": "success",
  "data": {
    "Info": {
      "ID": "3EB0C5A277F7F9B6C599",
      "Type": "ReactionMessage"
    }
  }
}
```

**Exemplo cURL**:
```bash
# Adicionar reação
curl -X POST http://localhost:4000/message/react \
  -H "Content-Type: application/json" \
  -H "apikey: SUA-CHAVE-API" \
  -d '{
    "number": "5511999999999",
    "reaction": "❤️",
    "id": "3EB0C5A277F7F9B6C599",
    "fromMe": false
  }'

# Remover reação
curl -X POST http://localhost:4000/message/react \
  -H "Content-Type: application/json" \
  -H "apikey: SUA-CHAVE-API" \
  -d '{
    "number": "5511999999999",
    "reaction": "remove",
    "id": "3EB0C5A277F7F9B6C599",
    "fromMe": false
  }'
```

---

### Marcar como Lida

Marca mensagem(ns) como lida(s).

**Endpoint**: `POST /message/markread`

**Body**:
```json
{
  "number": "5511999999999",
  "id": [
    "3EB0C5A277F7F9B6C599",
    "3EB0C5A277F7F9B6C600"
  ]
}
```

**Parâmetros**:

| Campo | Tipo | Obrigatório | Descrição |
|-------|------|-------------|-----------|
| `number` | string | ✅ Sim | Número do chat |
| `id` | array | ✅ Sim | Array de IDs de mensagens para marcar como lidas |

**Resposta de Sucesso (200)**:
```json
{
  "message": "success",
  "data": {
    "timestamp": "2025-11-11T10:30:00Z"
  }
}
```

**Exemplo cURL**:
```bash
curl -X POST http://localhost:4000/message/markread \
  -H "Content-Type: application/json" \
  -H "apikey: SUA-CHAVE-API" \
  -d '{
    "number": "5511999999999",
    "id": ["3EB0C5A277F7F9B6C599"]
  }'
```

---

### Editar Mensagem

Edita o conteúdo de uma mensagem enviada.

**Endpoint**: `POST /message/edit`

**Body**:
```json
{
  "chat": "5511999999999@s.whatsapp.net",
  "messageId": "3EB0C5A277F7F9B6C599",
  "message": "Texto editado da mensagem"
}
```

**Parâmetros**:

| Campo | Tipo | Obrigatório | Descrição |
|-------|------|-------------|-----------|
| `chat` | string | ✅ Sim | JID do chat |
| `messageId` | string | ✅ Sim | ID da mensagem a editar |
| `message` | string | ✅ Sim | Novo texto da mensagem |

**Nota**: Só é possível editar mensagens de texto enviadas por você (fromMe=true).

**Resposta de Sucesso (200)**:
```json
{
  "message": "success",
  "data": {
    "messageId": "3EB0C5A277F7F9B6C599",
    "timestamp": "2025-11-11T10:30:00Z"
  }
}
```

**Exemplo cURL**:
```bash
curl -X POST http://localhost:4000/message/edit \
  -H "Content-Type: application/json" \
  -H "apikey: SUA-CHAVE-API" \
  -d '{
    "chat": "5511999999999@s.whatsapp.net",
    "messageId": "3EB0C5A277F7F9B6C599",
    "message": "Mensagem corrigida"
  }'
```

---

### Deletar Mensagem

Deleta uma mensagem para todos (revoke).

**Endpoint**: `POST /message/delete`

**Body**:
```json
{
  "chat": "5511999999999@s.whatsapp.net",
  "messageId": "3EB0C5A277F7F9B6C599"
}
```

**Parâmetros**:

| Campo | Tipo | Obrigatório | Descrição |
|-------|------|-------------|-----------|
| `chat` | string | ✅ Sim | JID do chat |
| `messageId` | string | ✅ Sim | ID da mensagem a deletar |

**Nota**: Só é possível deletar mensagens enviadas por você. O WhatsApp tem limite de tempo para deletar mensagens (geralmente até 1 hora).

**Resposta de Sucesso (200)**:
```json
{
  "message": "success",
  "data": {
    "messageId": "3EB0C5A277F7F9B6C599",
    "timestamp": "2025-11-11T10:30:00Z"
  }
}
```

**Exemplo cURL**:
```bash
curl -X POST http://localhost:4000/message/delete \
  -H "Content-Type: application/json" \
  -H "apikey: SUA-CHAVE-API" \
  -d '{
    "chat": "5511999999999@s.whatsapp.net",
    "messageId": "3EB0C5A277F7F9B6C599"
  }'
```

---

### Encaminhar Mensagem

Encaminha uma mensagem existente para outro chat, com o rótulo **"Encaminhada"** exibido no destinatário (igual ao encaminhamento nativo do WhatsApp).

**Endpoint**: `POST /message/forward`

**Body**:
```json
{
  "number": "5511999999999",
  "message": {
    "extendedTextMessage": {
      "text": "Texto da mensagem original"
    }
  },
  "id": "msg-custom-123",
  "delay": 1000,
  "formatJid": true,
  "forwardingScore": 1
}
```

**Parâmetros**:

| Campo | Tipo | Obrigatório | Descrição |
|-------|------|-------------|-----------|
| `number` | string | ✅ Sim | Destino do encaminhamento (usuário, grupo `@g.us` ou newsletter) |
| `message` | object | ✅ Sim | Payload completo da mensagem original (`waE2E.Message`) — o objeto `data.Message` recebido no webhook ou retornado pelos endpoints `/send/*` |
| `id` | string | ❌ Não | ID customizado para a cópia encaminhada |
| `delay` | int32 | ❌ Não | Delay em milissegundos ("digitando...") antes de enviar |
| `formatJid` | bool | ❌ Não | Formatar número automaticamente (padrão: true) |
| `forwardingScore` | uint32 | ❌ Não | Sobrescreve o score calculado (padrão: score original + 1; valores ≥ 5 exibem "Encaminhada muitas vezes") |

**Como funciona**:

- O `message` deve ser enviado **exatamente como recebido** no webhook (`data.Message`) ou no response de um `/send/*`. O endpoint clona o payload, descarta o contexto antigo (citações/menções do chat de origem) e carimba `isForwarded: true` + `forwardingScore`.
- **Mídia é encaminhada por referência** (`mediaKey`/`directPath` reaproveitados), sem download nem re-upload — por isso é importante enviar o objeto de mídia completo (`URL`, `mediaKey`, `directPath`, `fileSHA256` etc.).
- Tipos suportados: texto (`conversation`/`extendedTextMessage`), imagem, vídeo, PTV, áudio, documento, sticker, localização, contato, enquete, lista, botões e interactive.
- Mensagens **view-once** (visualização única) não podem ser encaminhadas (retorna erro).

**Resposta de Sucesso (200)**: mesmo formato dos endpoints `/send/*`:
```json
{
  "message": "success",
  "data": {
    "Info": {
      "ID": "3EB0C5A277F7F9B6C599",
      "Timestamp": "2025-11-11T10:30:00Z",
      "Type": "ExtendedTextMessage"
    },
    "Message": {
      "extendedTextMessage": {
        "text": "Texto da mensagem original",
        "contextInfo": {
          "forwardingScore": 1,
          "isForwarded": true
        }
      }
    }
  }
}
```

**Respostas de Erro (400)**:
```json
{ "error": "phone number is required" }
```
```json
{ "error": "message payload is required" }
```
```json
{ "error": "view-once messages cannot be forwarded" }
```
```json
{ "error": "unsupported message type for forwarding" }
```

**Exemplo cURL (texto)**:
```bash
curl -X POST http://localhost:4000/message/forward \
  -H "Content-Type: application/json" \
  -H "apikey: SUA-CHAVE-API" \
  -d '{
    "number": "5511999999999",
    "message": {
      "extendedTextMessage": {
        "text": "Teste encaminhamento 1"
      }
    }
  }'
```

**Exemplo cURL (imagem recebida via webhook)**:
```bash
curl -X POST http://localhost:4000/message/forward \
  -H "Content-Type: application/json" \
  -H "apikey: SUA-CHAVE-API" \
  -d '{
    "number": "5511888888888",
    "message": {
      "imageMessage": {
        "URL": "https://mmg.whatsapp.net/...",
        "mimetype": "image/jpeg",
        "fileSHA256": "...",
        "fileLength": 12345,
        "mediaKey": "...",
        "fileEncSHA256": "...",
        "directPath": "/v/..."
      }
    }
  }'
```

---

### Presença no Chat

Define o status de presença no chat (digitando, gravando áudio, online).

**Endpoint**: `POST /message/presence`

**Body**:
```json
{
  "number": "5511999999999",
  "state": "composing",
  "isAudio": false
}
```

**Parâmetros**:

| Campo | Tipo | Obrigatório | Descrição |
|-------|------|-------------|-----------|
| `number` | string | ✅ Sim | Número do chat |
| `state` | string | ✅ Sim | Estado: `composing`, `paused`, `recording`, `available`, `unavailable` |
| `isAudio` | bool | ❌ Não | Se true, mostra "gravando áudio" (apenas com state=composing) |

**Estados Disponíveis**:
- `composing` - Digitando...
- `paused` - Para de digitar
- `recording` - Gravando áudio (use isAudio=true)
- `available` - Online
- `unavailable` - Offline

**Resposta de Sucesso (200)**:
```json
{
  "message": "success",
  "data": {
    "timestamp": "2025-11-11T10:30:00Z"
  }
}
```

**Exemplo cURL**:
```bash
# Mostrar "digitando..."
curl -X POST http://localhost:4000/message/presence \
  -H "Content-Type: application/json" \
  -H "apikey: SUA-CHAVE-API" \
  -d '{
    "number": "5511999999999",
    "state": "composing",
    "isAudio": false
  }'

# Mostrar "gravando áudio..."
curl -X POST http://localhost:4000/message/presence \
  -H "Content-Type: application/json" \
  -H "apikey: SUA-CHAVE-API" \
  -d '{
    "number": "5511999999999",
    "state": "composing",
    "isAudio": true
  }'

# Parar de digitar
curl -X POST http://localhost:4000/message/presence \
  -H "Content-Type: application/json" \
  -H "apikey: SUA-CHAVE-API" \
  -d '{
    "number": "5511999999999",
    "state": "paused"
  }'
```

---

### Download de Mídia

Faz download de mídia de uma mensagem recebida e retorna em base64.

**Endpoint**: `POST /message/downloadimage`

**Body**:
```json
{
  "message": {
    "imageMessage": {
      "url": "...",
      "mimetype": "image/jpeg",
      "fileSha256": "...",
      "fileLength": "..."
    }
  }
}
```

**Parâmetros**:

| Campo | Tipo | Obrigatório | Descrição |
|-------|------|-------------|-----------|
| `message` | object | ✅ Sim | Objeto de mensagem completo (do webhook) |

**Tipos de Mídia Suportados**:
- `imageMessage` - Imagens
- `videoMessage` - Vídeos
- `audioMessage` - Áudios
- `documentMessage` - Documentos
- `stickerMessage` - Stickers (convertido para PNG)

**Nota**: O objeto `message` deve ser o mesmo recebido via webhook/event. Contém todas as informações necessárias para download (URL, chaves de criptografia, etc).

**Resposta de Sucesso (200)**:
```json
{
  "message": "success",
  "data": {
    "base64": "data:image/jpeg;base64,/9j/4AAQSkZJRg...",
    "timestamp": "2025-11-11T10:30:00Z"
  }
}
```

**Exemplo cURL**:
```bash
curl -X POST http://localhost:4000/message/downloadimage \
  -H "Content-Type: application/json" \
  -H "apikey: SUA-CHAVE-API" \
  -d '{
    "message": {
      "imageMessage": {
        "url": "https://mmg.whatsapp.net/...",
        "mimetype": "image/jpeg",
        "fileSha256": "...",
        "fileLength": 123456
      }
    }
  }'
```

---

### Status da Mensagem

Consulta o status de entrega/leitura de uma mensagem no banco de dados.

**Endpoint**: `POST /message/status`

**Body**:
```json
{
  "id": "3EB0C5A277F7F9B6C599"
}
```

**Parâmetros**:

| Campo | Tipo | Obrigatório | Descrição |
|-------|------|-------------|-----------|
| `id` | string | ✅ Sim | ID da mensagem |

**Nota**: Requer `DATABASE_SAVE_MESSAGES=true` para funcionar. O sistema precisa estar salvando mensagens no banco.

**Resposta de Sucesso (200)**:
```json
{
  "message": "success",
  "data": {
    "result": {
      "id": "3EB0C5A277F7F9B6C599",
      "remoteJid": "5511999999999@s.whatsapp.net",
      "fromMe": true,
      "messageType": "conversation",
      "status": "READ",
      "timestamp": "2025-11-11T10:30:00Z"
    },
    "timestamp": "2025-11-11T10:31:00Z"
  }
}
```

**Status Possíveis**:
- `PENDING` - Enviando
- `SENT` - Enviada
- `DELIVERED` - Entregue
- `READ` - Lida

**Exemplo cURL**:
```bash
curl -X POST http://localhost:4000/message/status \
  -H "Content-Type: application/json" \
  -H "apikey: SUA-CHAVE-API" \
  -d '{
    "id": "3EB0C5A277F7F9B6C599"
  }'
```

---

## Recursos Adicionais

### Citação de Mensagens (Quoted)

Para citar/responder uma mensagem, adicione o objeto `quoted` em qualquer endpoint de envio:

```json
{
  "number": "5511999999999",
  "text": "Respondendo sua mensagem",
  "quoted": {
    "messageId": "3EB0C5A277F7F9B6C599",
    "participant": "5511999999999@s.whatsapp.net"
  }
}
```

### Menções em Grupos

Para mencionar usuários em grupos, use `mentionedJid` ou `mentionAll`:

```json
{
  "number": "120363XXXXXXXXXX@g.us",
  "text": "Olá @usuario, tudo bem?",
  "mentionedJid": "5511888888888@s.whatsapp.net"
}
```

Ou mencionar todos:

```json
{
  "number": "120363XXXXXXXXXX@g.us",
  "text": "@todos Reunião às 15h!",
  "mentionAll": true
}
```

### Delay e Presença

Simule digitação antes de enviar:

```json
{
  "number": "5511999999999",
  "text": "Mensagem com delay",
  "delay": 3000
}
```

Isso mostrará "digitando..." por 3 segundos antes de enviar a mensagem.

### Verificação de Número

Por padrão, o sistema verifica se o número existe no WhatsApp antes de enviar (configurável via `CHECK_USER_EXISTS`). Se desabilitado, mensagens podem falhar silenciosamente.

### Formatação de Números

O parâmetro `formatJid` (padrão: true) normaliza automaticamente o número:
- Remove caracteres especiais
- Adiciona sufixo @s.whatsapp.net
- Valida formato

Para enviar para JIDs já formatados (grupos, etc), use `formatJid: false`.

---

## Códigos de Erro Comuns

| Código | Erro | Solução |
|--------|------|---------|
| 400 | `phone number is required` | Forneça o campo `number` |
| 400 | `message body is required` | Forneça o campo `text` ou conteúdo |
| 400 | `minimum 2 options are required` | Enquetes precisam de pelo menos 2 opções |
| 400 | `Invalid file format` | Formato de arquivo não suportado |
| 500 | `instance not found` | Instância não existe ou não está conectada |
| 500 | `client disconnected` | Instância desconectada, reconecte |
| 500 | `number X is not registered on WhatsApp` | Número não existe no WhatsApp |

---

## Boas Práticas

### 1. Usar Delay em Múltiplas Mensagens
Ao enviar várias mensagens seguidas, use o parâmetro `delay` para parecer mais natural:
- Primeira mensagem: `"delay": 1000` (1 segundo)
- Segunda mensagem: `"delay": 2000` (2 segundos)
- Terceira mensagem: `"delay": 1500` (1.5 segundos)

Isso simula o tempo que uma pessoa levaria para digitar cada mensagem.

### 2. Verificar Status de Conexão
Antes de enviar mensagens em massa, verifique se a instância está conectada:
```bash
curl "http://localhost:4000/instance/status" \
  -H "apikey: TOKEN-DA-INSTANCIA"
```

### 3. Tratamento de Erros
Sempre trate erros HTTP 4xx (validação) e 5xx (servidor):
- **400**: Erro de validação (campos obrigatórios faltando, formato inválido)
- **500**: Erro no servidor (instância desconectada, número inválido, etc)

Sempre verifique o status code da resposta e o campo `error` no JSON retornado.

### 4. Usar Webhooks
Para receber mensagens, configure webhooks em vez de polling:
```env
WEBHOOK_URL=https://seu-servidor.com/webhook
```

### 5. Gerenciar Mídias
Para áudio, configure conversor externo para melhor performance:
```env
API_AUDIO_CONVERTER=https://seu-conversor.com/convert
API_AUDIO_CONVERTER_KEY=sua-chave
```

---

## Próximos Passos

- [API de Usuários](./api-user.md) - Gerenciar perfil e contatos
- [API de Grupos](./api-groups.md) - Criar e administrar grupos
- [Sistema de Eventos](../recursos-avancados/events-system.md) - Receber webhooks
- [Referência Completa da API](../guias-api/api-overview.md)

---

**Documentação gerada para Evolution GO v1.0**
