# Sistema de Eventos

Sistema completo de notificações em tempo real do Evolution GO. Receba eventos do WhatsApp através de múltiplos canais: Webhooks, RabbitMQ, NATS e WebSocket.

## 📋 Índice

- [Visão Geral](#visão-geral)
- [Canais Disponíveis](#canais-disponíveis)
- [Webhook](#webhook)
- [RabbitMQ](#rabbitmq)
- [NATS](#nats)
- [WebSocket](#websocket)
- [Configuração](#configuração)
- [Tipos de Eventos](#tipos-de-eventos)
- [Formato de Payload](#formato-de-payload)
- [Múltiplos Canais Simultâneos](#múltiplos-canais-simultâneos)
- [Exemplos Práticos](#exemplos-práticos)

---

## Visão Geral

O Evolution GO envia notificações de eventos do WhatsApp em tempo real através de diferentes canais de comunicação:

- **Webhook (HTTP POST)**: Ideal para integração simples com APIs externas
- **RabbitMQ (AMQP)**: Message broker para arquiteturas empresariais e filas confiáveis
- **NATS**: Message broker leve para comunicação de alta performance
- **WebSocket**: Conexão persistente para dashboards e aplicações web

Você pode ativar múltiplos canais simultaneamente - os eventos serão enviados para todos os canais configurados.

### Arquitetura

```
┌─────────────────────────────────────────────────────────┐
│                    WhatsApp Event                        │
│                  (mensagem recebida)                     │
└────────────────────┬────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────┐
│               Whatsmeow Event Handler                    │
│         (detecta evento do WhatsApp)                     │
└────────────────────┬────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────┐
│                Event Router/Dispatcher                   │
│           (filtra eventos subscritos)                    │
└─────┬──────┬──────┬──────┬────────────────────────────┘
      │      │      │      │
      │      │      │      │
      ▼      ▼      ▼      ▼
   ┌────┐ ┌────┐ ┌────┐ ┌─────┐
   │Web │ │AMQP│ │NATS│ │Web  │
   │hook│ │MQ  │ │    │ │Socket│
   └─┬──┘ └─┬──┘ └─┬──┘ └──┬──┘
     │      │      │       │
     │      │      │       │
     ▼      ▼      ▼       ▼
  ┌────┐ ┌────┐ ┌────┐ ┌─────┐
  │HTTP│ │Queue│ │Topic│ │WS   │
  │POST│ │     │ │     │ │Conn │
  └────┘ └────┘ └────┘ └─────┘
```

---

## Canais Disponíveis

### Comparação de Canais

| Produtor | Latência | Throughput | Persistência | Complexidade | Caso de Uso |
|----------|----------|------------|--------------|--------------|-------------|
| **Webhook** | Baixa | Média | Não | Baixa | Integração simples com APIs |
| **RabbitMQ** | Média | Alta | Sim | Alta | Arquiteturas distribuídas, filas |
| **NATS** | Muito Baixa | Muito Alta | Opcional | Média | Real-time, pub/sub, microserviços |
| **WebSocket** | Muito Baixa | Alta | Não | Média | Aplicações web, dashboards |

---

## Webhook

### Visão Geral

Envia eventos via HTTP POST para uma URL que você configurar. É o método mais simples para integrar o Evolution GO com suas aplicações.

### Características

- **Retry automático**: 5 tentativas com intervalo de 30 segundos entre cada tentativa
- **Timeout**: Configurável
- **Content-Type**: `application/json`
- **Método**: HTTP POST

### Configuração

**Variáveis de Ambiente**:
```env
# Webhook global (recebe eventos de todas as instâncias)
WEBHOOK_URL=https://meu-servidor.com/webhook
```

**Por instância** (via Connect):
```bash
curl -X POST http://localhost:4000/instance/connect \
  -H "apikey: token-da-instancia" \
  -H "Content-Type: application/json" \
  -d '{
    "webhookUrl": "https://meu-servidor.com/webhook-vendas",
    "subscribe": ["MESSAGE", "READ_RECEIPT", "GROUP", "CALL"],
    "rabbitmqEnabled": "disabled",
    "websocketEnable": "disabled",
    "natsEnabled": "disabled"
  }'
```

> **Nota**: A instância é identificada pelo token no header `apikey`, não por `instanceName` no body.

### Funcionamento

Quando um evento ocorre no WhatsApp:

1. **Webhook Global**: Se configurado via `WEBHOOK_URL`, todos os eventos são enviados para esta URL
2. **Webhook por Instância**: Se configurado no `POST /instance/connect`, eventos daquela instância vão para a URL específica
3. **Retry Automático**: Se a requisição falhar, o Evolution GO tenta novamente até 5 vezes
4. **Intervalo**: 30 segundos entre cada tentativa

### Requisição HTTP

**Headers**:
```
POST /webhook HTTP/1.1
Host: meu-servidor.com
Content-Type: application/json
```

**Body**:
```json
{
  "event": "MESSAGE",
  "instance": "vendas",
  "data": {
    "key": {
      "remoteJid": "5511999999999@s.whatsapp.net",
      "fromMe": false,
      "id": "3EB0C5A277F7F9B6C599"
    },
    "message": {
      "conversation": "Olá!"
    },
    "messageTimestamp": "1699999999"
  }
}
```

### Implementação no Servidor Receptor

**Node.js (Express)**:
```javascript
const express = require('express');
const app = express();

app.use(express.json());

app.post('/webhook', (req, res) => {
    const { event, instance, data } = req.body;
    
    console.log(`Evento recebido: ${event} da instância ${instance}`);
    console.log('Dados:', JSON.stringify(data, null, 2));
    
    // Processa evento
    if (event === 'MESSAGE') {
        const message = data.message.conversation;
        const from = data.key.remoteJid;
        console.log(`Mensagem de ${from}: ${message}`);
    }
    
    // IMPORTANTE: Retorne 200 OK rapidamente
    res.status(200).json({ received: true });
});

app.listen(3000, () => console.log('Webhook server rodando na porta 3000'));
```

**Python (Flask)**:
```python
from flask import Flask, request, jsonify

app = Flask(__name__)

@app.route('/webhook', methods=['POST'])
def webhook():
    data = request.json
    event = data.get('event')
    instance = data.get('instance')
    payload = data.get('data')
    
    print(f"Evento recebido: {event} da instância {instance}")
    
    if event == 'MESSAGE':
        message = payload.get('message', {}).get('conversation')
        from_jid = payload.get('key', {}).get('remoteJid')
        print(f"Mensagem de {from_jid}: {message}")
    
    return jsonify({'received': True}), 200

if __name__ == '__main__':
    app.run(port=3000)
```

---

## RabbitMQ

### Visão Geral

Publica eventos em filas RabbitMQ (AMQP). Ideal para arquiteturas distribuídas que precisam de filas confiáveis e persistentes.

### Características

- **Filas duráveis**: Mensagens não se perdem mesmo após reinicialização do servidor
- **Alta disponibilidade**: Replicação automática entre nós
- **Retry automático**: 3 tentativas com intervalo crescente
- **Confirmações de entrega**: Garantia de que a mensagem foi recebida
- **Heartbeat**: Monitoramento de conexão a cada 30 segundos
- **Reconexão automática**: Reconecta automaticamente em caso de falha

### Configuração

**Variáveis de Ambiente**:
```env
# URL de conexão RabbitMQ
AMQP_URL=amqp://user:password@localhost:5672/

# Habilitar RabbitMQ global
AMQP_GLOBAL_ENABLED=true

# Eventos globais (modo fallback)
AMQP_GLOBAL_EVENTS=MESSAGE,SEND_MESSAGE,GROUP,CALL

# Eventos específicos (prioridade sobre global)
AMQP_SPECIFIC_EVENTS=message,sendmessage,groupinfo,calloffer
```

### Tipos de Filas

#### Modo 1: AMQP_SPECIFIC_EVENTS (Recomendado)

Cria filas específicas para eventos exatos:

```env
AMQP_SPECIFIC_EVENTS=message,sendmessage,receipt,presence
```

**Filas criadas**:
- `message` - Mensagens recebidas
- `sendmessage` - Mensagens enviadas
- `receipt` - Confirmações de leitura
- `presence` - Status online/offline

#### Modo 2: AMQP_GLOBAL_EVENTS (Fallback)

Mapeia eventos globais para múltiplas filas:

```env
AMQP_GLOBAL_EVENTS=MESSAGE,CALL,CONNECTION
```

**Mapeamento de Eventos**:
- `MESSAGE` → fila `message`
- `SEND_MESSAGE` → fila `sendmessage`
- `READ_RECEIPT` → fila `receipt`
- `PRESENCE` → fila `presence`
- `CALL` → filas `calloffer`, `callaccept`, `callterminate`
- `CONNECTION` → filas `connected`, `disconnected`, `loggedout`
- `GROUP` → filas `groupinfo`, `joinedgroup`
- `QRCODE` → filas `qrcode`, `qrtimeout`, `qrsuccess`

### Propriedades das Filas

As filas RabbitMQ criadas pelo Evolution GO são configuradas com:

- **Quorum queues**: Replicação automática para alta disponibilidade
- **Durabilidade**: Mensagens persistem após restart do servidor
- **Persistência**: Todas as mensagens são marcadas como persistentes

### Consumindo Mensagens

**Go (amqp091-go)**:
```go
package main

import (
    "fmt"
    "log"
    amqp "github.com/rabbitmq/amqp091-go"
)

func main() {
    conn, err := amqp.Dial("amqp://user:password@localhost:5672/")
    if err != nil {
        log.Fatal(err)
    }
    defer conn.Close()

    ch, err := conn.Channel()
    if err != nil {
        log.Fatal(err)
    }
    defer ch.Close()

    msgs, err := ch.Consume(
        "message",  // queue
        "",         // consumer
        true,       // auto-ack
        false,      // exclusive
        false,      // no-local
        false,      // no-wait
        nil,        // args
    )
    if err != nil {
        log.Fatal(err)
    }

    forever := make(chan bool)

    go func() {
        for d := range msgs {
            fmt.Printf("Recebido: %s
", d.Body)
        }
    }()

    fmt.Println("Aguardando mensagens...")
    <-forever
}
```

**Python (pika)**:
```python
import pika
import json

connection = pika.BlockingConnection(
    pika.ConnectionParameters('localhost'))
channel = connection.channel()

def callback(ch, method, properties, body):
    data = json.loads(body)
    print(f"Evento recebido: {data}")
    
    if data.get('event') == 'MESSAGE':
        message = data.get('data', {}).get('message', {}).get('conversation')
        print(f"Mensagem: {message}")

channel.basic_consume(queue='message', on_message_callback=callback, auto_ack=True)

print('Aguardando mensagens...')
channel.start_consuming()
```

---

## NATS

### Visão Geral

Publica eventos em tópicos NATS. Ideal para comunicação em tempo real com latência mínima.

### Características

- **Latência ultra-baixa**: Comunicação extremamente rápida
- **Pub/Sub nativo**: Vários consumidores podem receber o mesmo evento
- **Leve e rápido**: Menor overhead que RabbitMQ
- **Clustering**: Suporte nativo a clusters distribuídos

### Configuração

```env
# URL de conexão NATS
NATS_URL=nats://localhost:4222

# Habilitar NATS
NATS_ENABLED=true
```

### Tópicos

Eventos são publicados em tópicos no formato:

```
evolution.{instance}.{event_type}
```

**Exemplos**:
- `evolution.vendas.message` - Mensagens da instância "vendas"
- `evolution.suporte.calloffer` - Chamadas da instância "suporte"
- `evolution.*.message` - Mensagens de todas as instâncias (wildcard)

### Consumindo Eventos

**Go (nats.go)**:
```go
package main

import (
    "fmt"
    "log"
    "github.com/nats-io/nats.go"
)

func main() {
    nc, err := nats.Connect("nats://localhost:4222")
    if err != nil {
        log.Fatal(err)
    }
    defer nc.Close()

    // Subscrever a eventos de mensagens de todas as instâncias
    sub, err := nc.Subscribe("evolution.*.message", func(m *nats.Msg) {
        fmt.Printf("Recebido no tópico %s: %s
", m.Subject, string(m.Data))
    })
    if err != nil {
        log.Fatal(err)
    }
    defer sub.Unsubscribe()

    // Manter rodando
    select {}
}
```

---

## WebSocket

### Visão Geral

Envia eventos através de uma conexão WebSocket persistente. Ideal para dashboards e aplicações web que precisam de atualizações em tempo real.

### Características

- **Eventos do servidor**: O cliente recebe eventos; mensagens enviadas pelo cliente são descartadas
- **Baixa latência**: Perfeito para interfaces em tempo real
- **Gerenciamento seguro**: Múltiplas conexões simultâneas
- **Dois modos**: Broadcast (todos os eventos) ou específico por instância

### Tipos de Conexão

#### 1. Conexão Específica (Por Instância)

Recebe apenas eventos de uma instância:

```
ws://localhost:8080/ws?token=GLOBAL_API_KEY&instanceId=UUID_DA_INSTANCIA
```

Nos dois modos, `/ws` exige a chave global (`GLOBAL_API_KEY`), inclusive quando
`instanceId` está presente. Use o UUID retornado pela criação da instância, não
seu nome. Ajuste a porta conforme `SERVER_PORT` (o `.env.example` usa `8080`).

#### 2. Conexão Broadcast

Recebe eventos de **todas as instâncias**:

```
ws://localhost:8080/ws?token=GLOBAL_API_KEY
```

### Gerenciamento de Conexões

O Evolution GO gerencia automaticamente as conexões WebSocket:

- **Conexões específicas**: Uma conexão ativa por instância; uma nova conexão substitui e fecha a anterior
- **Conexões broadcast**: Recebem eventos de todas as instâncias
- **Limpeza automática**: Desconexões e falhas de escrita removem e fecham a conexão correspondente, sem remover uma reconexão mais recente
- **Escritas serializadas**: Eventos concorrentes compartilham um único writer por conexão
- **Prazo de escrita**: Cada escrita tem prazo de 10 segundos; uma falha ou timeout fecha o socket e exige uma nova conexão
- **Registro independente do envio**: Um cliente lento não mantém o registro global bloqueado durante operações de rede

A entrega é por tentativa, sem fila persistente, repetição automática ou replay
após reconectar. Uma falha não interrompe as tentativas para os demais destinatários
do mesmo evento. `Produce` retorna erro quando algum envio falha, mesmo que outros
clientes tenham recebido o evento; repetir toda a publicação pode duplicá-lo.
No despacho da instância, uma falha de WebSocket é registrada e o webhook
configurado continua sendo tentado, sem repetir a publicação WebSocket.
O formato permanece `{ "queue": "nome-em-minusculas", "payload": "JSON como string" }`.

### Cliente JavaScript

```javascript
// Conectar a instância específica
const token = 'GLOBAL_API_KEY';
const instanceId = 'UUID_DA_INSTANCIA';
const ws = new WebSocket(`ws://localhost:8080/ws?token=${encodeURIComponent(token)}&instanceId=${encodeURIComponent(instanceId)}`);

ws.onopen = () => {
    console.log('WebSocket conectado!');
};

ws.onmessage = (event) => {
    const data = JSON.parse(event.data);
    console.log('Evento recebido:', data.queue);
    console.log('Payload:', JSON.parse(data.payload));
    
    if (data.queue === 'message') {
        const message = JSON.parse(data.payload);
        console.log('Nova mensagem:', message.data.message.conversation);
    }
};

ws.onerror = (error) => {
    console.error('Erro no WebSocket:', error);
};

ws.onclose = () => {
    console.log('WebSocket desconectado');
    // Reconectar após 5 segundos
    setTimeout(() => {
        console.log('Reconectando...');
        // Criar nova conexão
    }, 5000);
};
```

### Cliente Python

```python
import asyncio
import websockets
import json

async def listen_events():
    uri = "ws://localhost:8080/ws?token=GLOBAL_API_KEY&instanceId=UUID_DA_INSTANCIA"
    
    async with websockets.connect(uri) as websocket:
        print("WebSocket conectado!")
        
        async for message in websocket:
            data = json.loads(message)
            queue = data.get('queue')
            payload = json.loads(data.get('payload'))
            
            print(f"Evento: {queue}")
            
            if queue == 'message':
                msg_text = payload.get('data', {}).get('message', {}).get('conversation')
                print(f"Mensagem recebida: {msg_text}")

asyncio.run(listen_events())
```

---

## Configuração

### Exemplo Completo (.env)

```env
# ===== WEBHOOK =====
WEBHOOK_URL=https://meu-servidor.com/webhook-global

# ===== RABBITMQ =====
AMQP_URL=amqp://admin:password@rabbitmq:5672/
AMQP_GLOBAL_ENABLED=true
AMQP_SPECIFIC_EVENTS=message,sendmessage,receipt,presence,calloffer

# ===== NATS =====
NATS_URL=nats://nats:4222
NATS_ENABLED=true

# ===== WEBSOCKET =====
# WebSocket é habilitado automaticamente
# Acesse: ws://localhost:4000/ws
```

### Configuração por Instância

```bash
curl -X POST http://localhost:4000/instance/connect \
  -H "apikey: token-da-instancia-vendas" \
  -H "Content-Type: application/json" \
  -d '{
    "webhookUrl": "https://meu-servidor.com/webhook-vendas",
    "subscribe": ["MESSAGE", "GROUP", "CALL"],
    "rabbitmqEnabled": "enabled",
    "websocketEnable": "disabled",
    "natsEnabled": "enabled"
  }'
```

**Comportamento**:
- `rabbitmqEnabled: "enabled"` ou `"disabled"` - Habilita/desabilita RabbitMQ para esta instância
- `websocketEnable: "disabled"` ou outro valor - Habilita/desabilita WebSocket
- `natsEnabled: "enabled"` ou `"disabled"` - Habilita/desabilita NATS
- `webhookUrl` - URL específica para esta instância
- `subscribe` - Array com categorias de eventos: `MESSAGE`, `READ_RECEIPT`, `PRESENCE`, `HISTORY_SYNC`, `CHAT_PRESENCE`, `CALL`, `CONNECTION`, `QRCODE`, `LABEL`, `CONTACT`, `GROUP`, `NEWSLETTER`, ou `ALL`

---

## Tipos de Eventos

### Categorias vs Eventos Individuais

O Evolution GO usa dois níveis de classificação de eventos:

**Categorias de Eventos** (usadas no `subscribe`):
- São escritas em **MAIÚSCULAS**: `MESSAGE`, `GROUP`, `CALL`, etc.
- Agrupam múltiplos eventos relacionados
- Usadas para filtrar quais categorias você quer receber no `subscribe`

**Eventos Individuais** (nomes dos eventos emitidos):
- São escritos em **minúsculas**: `message`, `groupinfo`, `calloffer`, etc.
- São os eventos específicos que você receberá no webhook/fila
- Cada categoria pode gerar múltiplos eventos individuais

**Exemplo de mapeamento**:

| Categoria (`subscribe`) | Eventos Individuais Emitidos |
|------------------------|------------------------------|
| `MESSAGE` | `Message` (evento recebido no webhook) |
| `SEND_MESSAGE` | `SendMessage` |
| `READ_RECEIPT` | `Receipt` |
| `GROUP` | `GroupInfo`, `JoinedGroup` |
| `CALL` | `CallOffer`, `CallAccept`, `CallTerminate` |
| `CONNECTION` | `Connected`, `Disconnected`, `LoggedOut` |
| `QRCODE` | `PairSuccess`, `Disconnected` (com QR) |
| `ALL` | Todos os eventos |

**Exemplo prático**:
```json
// No connect, você filtra por categoria:
{
  "subscribe": ["MESSAGE", "GROUP", "CALL"]
}

// No webhook, você recebe o evento individual:
{
  "event": "Message",  // ou "GroupInfo", "CallOffer", etc
  "data": { ... }
}
```

### Eventos de Mensagens

**Categoria**: `MESSAGE` e `SEND_MESSAGE`

- `Message` - Mensagem recebida
- `SendMessage` - Mensagem enviada
- `Receipt` - Confirmação de leitura (`READ_RECEIPT`)
- Reações, edições, deleções de mensagens

### Eventos de Grupos

**Categoria**: `GROUP`

- `GroupInfo` - Informações do grupo atualizadas (nome, descrição, participantes)
- `JoinedGroup` - Bot adicionado a um grupo

### Eventos de Chamadas

**Categoria**: `CALL`

- `CallOffer` - Chamada recebida
- `CallAccept` - Chamada aceita
- `CallTerminate` - Chamada encerrada

### Eventos de Conexão

**Categoria**: `CONNECTION`

- `Connected` - Instância conectada com sucesso
- `Disconnected` - Instância desconectada
- `LoggedOut` - Logout realizado (por outra sessão ou WhatsApp)
- `TemporaryBan` - Conta temporariamente banida

### Eventos de QR Code

**Categoria**: `QRCODE`

- `PairSuccess` - QR Code escaneado com sucesso
- `Disconnected` - Desconectado (pode incluir informações de QR)

### Eventos de Presença

**Categoria**: `PRESENCE` e `CHAT_PRESENCE`

- `Presence` - Status online/offline de contato
- `ChatPresence` - Digitando/gravando áudio

### Eventos de Contatos

**Categoria**: `CONTACT`

- `Contact` - Informações de contato atualizadas
- `PushName` - Nome do contato alterado

### Eventos de Labels

**Categoria**: `LABEL`

- `LabelEdit` - Label criada/editada/deletada
- `LabelAssociationChat` - Label associada a chat
- `LabelAssociationMessage` - Label associada a mensagem

### Eventos de Newsletter

**Categoria**: `NEWSLETTER`

- `NewsletterJoin` - Inscrito em newsletter
- `NewsletterLeave` - Saiu de newsletter

### Sincronização de Histórico

**Categoria**: `HISTORY_SYNC`

- `HistorySync` - Sincronização de histórico do telefone
- `OfflineSyncCompleted` - Sincronização offline concluída

---

## Formato de Payload

> **📚 Exemplos Detalhados de Webhooks**
>
> Para exemplos completos de JSON de todos os eventos de webhook, consulte nossa documentação detalhada:
>
> 🔗 **[Webhook - Exemplos Completos (Notion)](https://atendai.notion.site/Webhook-11b50bf742da80d99acafe4d92ccd054?pvs=74)**
>
> Este guia contém exemplos reais de payload para todos os tipos de eventos, incluindo mensagens de texto, mídia, grupos, chamadas, e muito mais.

### Estrutura Padrão

```json
{
  "event": "MESSAGE",
  "instance": "nome-da-instancia",
  "data": {
    // ... dados específicos do evento
  }
}
```

### Exemplo: Mensagem Recebida

```json
{
  "event": "MESSAGE",
  "instance": "vendas",
  "data": {
    "key": {
      "remoteJid": "5511999999999@s.whatsapp.net",
      "fromMe": false,
      "id": "3EB0C5A277F7F9B6C599"
    },
    "message": {
      "conversation": "Olá! Gostaria de informações sobre produtos."
    },
    "messageTimestamp": "1699999999",
    "pushName": "João Silva"
  }
}
```

### Exemplo: Grupo Criado

```json
{
  "event": "GROUP_UPDATE",
  "instance": "suporte",
  "data": {
    "jid": "120363XXXXXXXXXX@g.us",
    "subject": "Suporte Técnico",
    "announce": false,
    "participants": [
      "5511999999999@s.whatsapp.net",
      "5511888888888@s.whatsapp.net"
    ]
  }
}
```

### Exemplo: Chamada Recebida

```json
{
  "event": "CALL",
  "instance": "vendas",
  "data": {
    "id": "ABC123",
    "from": "5511999999999@s.whatsapp.net",
    "timestamp": "1699999999",
    "isVideo": false,
    "isGroup": false
  }
}
```

---

## Múltiplos Canais Simultâneos

### Cenário: Todos os Canais Ativos

```env
# Habilitar tudo
WEBHOOK_URL=https://api.exemplo.com/webhook
AMQP_URL=amqp://localhost:5672/
AMQP_GLOBAL_ENABLED=true
NATS_URL=nats://localhost:4222
NATS_ENABLED=true
```

```bash
curl -X POST http://localhost:4000/instance/connect \
  -H "apikey: token-vendas" \
  -H "Content-Type: application/json" \
  -d '{
    "webhookUrl": "https://api.exemplo.com/webhook-vendas",
    "rabbitmqEnabled": "enabled",
    "websocketEnable": "disabled",
    "natsEnabled": "enabled",
    "subscribe": ["MESSAGE"]
  }'
```

**Resultado**: Ao receber 1 mensagem, o evento será enviado para:
1. ✅ Webhook global (`WEBHOOK_URL`)
2. ✅ Webhook da instância (`webhookUrl`)
3. ✅ Fila RabbitMQ `message`
4. ✅ Tópico NATS `evolution.vendas.message`
5. ✅ Clientes WebSocket conectados

**Total**: **5 destinos** para o mesmo evento!

---

## Exemplos Práticos

### 1. Setup Básico com Webhook

```bash
# .env
WEBHOOK_URL=https://meu-dominio.com/webhook

# Conectar instância
curl -X POST http://localhost:4000/instance/connect \
  -H "Content-Type: application/json" \
  -H "apikey: token-bot-123" \
  -d '{
    "subscribe": ["MESSAGE"]
  }'

# Servidor receptor
node webhook-server.js
```

### 2. Arquitetura Distribuída com RabbitMQ

```bash
# Docker Compose
docker run -d --name rabbitmq   -p 5672:5672   -p 15672:15672   rabbitmq:3-management

# .env
AMQP_URL=amqp://guest:guest@localhost:5672/
AMQP_GLOBAL_ENABLED=true
AMQP_SPECIFIC_EVENTS=message,sendmessage

# Consumidor em Python
python rabbitmq-consumer.py
```

### 3. Dashboard em Tempo Real com WebSocket

```html
<!DOCTYPE html>
<html>
<head>
    <title>Evolution GO Dashboard</title>
</head>
<body>
    <h1>Mensagens em Tempo Real</h1>
    <div id="messages"></div>

    <script>
        const ws = new WebSocket('ws://localhost:4000/ws?token=GLOBAL_API_KEY');
        
        ws.onmessage = (event) => {
            const data = JSON.parse(event.data);
            const payload = JSON.parse(data.payload);
            
            if (data.queue === 'message') {
                const message = payload.data.message.conversation;
                const from = payload.data.pushName;
                
                const div = document.getElementById('messages');
                div.innerHTML += `<p><strong>${from}:</strong> ${message}</p>`;
            }
        };
    </script>
</body>
</html>
```

### 4. Microserviços com NATS

```bash
# Serviço 1: Processar mensagens
nats-consumer-messages.py

# Serviço 2: Processar chamadas
nats-consumer-calls.py

# Serviço 3: Analytics
nats-consumer-analytics.py

# Todos consomem independentemente
```

---

## Boas Práticas

### 1. Escolha o Canal Adequado

**Webhook** é ideal para:
- Integração simples com APIs existentes
- Aplicações que não precisam de garantias de entrega complexas
- Quando você tem controle sobre o servidor receptor

**RabbitMQ** é ideal para:
- Arquiteturas distribuídas com múltiplos consumidores
- Quando precisa garantir que nenhuma mensagem seja perdida
- Sistemas que exigem alta disponibilidade

**NATS** é ideal para:
- Comunicação em tempo real com latência mínima
- Arquiteturas de microserviços
- Quando você precisa de alto volume de mensagens

**WebSocket** é ideal para:
- Dashboards e painéis de controle
- Aplicações web que mostram dados em tempo real
- Quando você precisa de atualização instantânea na interface

### 2. Subscreva Apenas aos Eventos Necessários

Configure apenas os eventos que sua aplicação realmente vai processar:

```javascript
// ❌ Evite: subscrever a todos os eventos
{"subscribe": ["ALL"]}

// ✅ Melhor: especifique apenas o que você precisa
{"subscribe": ["MESSAGE", "GROUP_UPDATE"]}
```

### 3. Implemente Idempotência

Em casos de retry, o mesmo evento pode ser recebido mais de uma vez. Implemente lógica para evitar processamento duplicado:

```python
processed_messages = set()

def process_event(message_id, content):
    # Verifica se já foi processado
    if message_id in processed_messages:
        print(f"Evento {message_id} já processado, ignorando duplicata")
        return
    
    # Processa o evento
    print(f"Processando evento: {content}")
    
    # Marca como processado
    processed_messages.add(message_id)
```

### 4. Monitore Filas

**RabbitMQ**:
```bash
# Verificar filas
rabbitmqctl list_queues name messages consumers

# Interface web
http://localhost:15672
```

**NATS**:
```bash
nats server list
nats sub "evolution.>"
```

### 5. Configure Dead Letter Queue

Para RabbitMQ, configure uma fila de mensagens com falha (Dead Letter Queue) para capturar eventos que não puderam ser processados após várias tentativas. Consulte a documentação do RabbitMQ para detalhes de configuração.

---

## Troubleshooting

### Webhook não está sendo chamado

**Diagnóstico**:
```bash
# Verificar logs da instância
GET /instance/logs/:instanceId?level=ERROR
```

**Causas comuns**:
1. URL inválida ou inacessível
2. Firewall bloqueando requisições
3. Servidor não retornando 2xx

**Solução**:
```bash
# Testar URL manualmente
curl -X POST https://seu-webhook.com/endpoint   -H "Content-Type: application/json"   -d '{"test": true}'
```

### RabbitMQ não conecta

**Erro**: `failed to connect to RabbitMQ`

**Soluções**:
1. Verificar se RabbitMQ está rodando: `docker ps | grep rabbitmq`
2. Testar conexão: `telnet localhost 5672`
3. Verificar credenciais em `AMQP_URL`

### WebSocket desconecta constantemente

**Causas**:
- Proxy/load balancer não suporta WebSocket
- Timeout de idle connection

**Solução (Nginx)**:
```nginx
location /ws {
    proxy_pass http://localhost:4000;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_read_timeout 86400;  # 24 horas
}
```

---

## Próximos Passos

- [Armazenamento de Mídia](./media-storage.md) - MinIO/S3 para arquivos
- [Conexão QR Code](./qrcode-connection.md) - Processo de autenticação
- [Multi-Dispositivo](./multi-device.md) - Suporte Multi-Device
- [API de Webhooks](../guias-api/api-webhooks.md) - Configurar webhooks via API

---

**Documentação gerada para Evolution GO v1.0**
