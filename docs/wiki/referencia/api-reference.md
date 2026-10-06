# Referência da API

Referência rápida de endpoints disponíveis na API Evolution GO.

## Autenticação

Todos os endpoints requerem header de autenticação:

```
apikey: SUA-CHAVE-API
```

## Base URL

```
http://localhost:4000
```

---

## Endpoints por Categoria

### Instâncias (16 endpoints)

**Gerenciamento (requer GLOBAL_API_KEY):**
- `POST /instance/create` - Criar nova instância
- `GET /instance/all` - Listar todas as instâncias
- `GET /instance/info/:instanceId` - Detalhes da instância
- `DELETE /instance/delete/:instanceId` - Remover instância

**Operações (requer token da instância):**
- `POST /instance/connect` - Iniciar conexão
- `GET /instance/status` - Status de conexão
- `GET /instance/qr` - Obter QR Code
- `POST /instance/pair` - Parear com código
- `POST /instance/disconnect` - Desconectar
- `POST /instance/reconnect` - Reconectar
- `DELETE /instance/logout` - Fazer logout

**Documentação completa:** [API de Instâncias](../guias-api/api-instances.md)

---

### Mensagens (17 endpoints)

**Envio:**
- `POST /send/text` - Mensagem de texto
- `POST /send/media` - Mídia (imagem/vídeo/áudio/documento)
- `POST /send/link` - Link com preview
- `POST /send/location` - Localização
- `POST /send/contact` - Contato
- `POST /send/poll` - Enquete
- `POST /send/sticker` - Sticker
- `POST /send/button` - Botões interativos
- `POST /send/list` - Lista de opções

**Operações:**
- `POST /message/react` - Reagir à mensagem
- `POST /message/markread` - Marcar como lida
- `POST /message/edit` - Editar mensagem
- `POST /message/delete` - Deletar mensagem
- `POST /message/forward` - Encaminhar mensagem (com rótulo "Encaminhada")
- `POST /message/presence` - Status de presença (digitando/gravando)
- `POST /message/downloadmedia` - Download de mídia
- `POST /message/status` - Status de entrega/leitura

**Documentação completa:** [API de Mensagens](../guias-api/api-messages.md)

---

### Usuários (13 endpoints)

- `POST /user/info` - Informações do usuário
- `POST /user/check` - Verificar número no WhatsApp
- `POST /user/avatar` - Obter avatar
- `GET /user/contacts` - Listar contatos
- `GET /user/privacy` - Configurações de privacidade
- `POST /user/privacy` - Atualizar privacidade
- `POST /user/block` - Bloquear contato
- `POST /user/unblock` - Desbloquear contato
- `GET /user/blocklist` - Lista de bloqueados
- `POST /user/profilePicture` - Atualizar foto de perfil
- `POST /user/profileName` - Atualizar nome
- `POST /user/profileStatus` - Atualizar status/recado

**Documentação completa:** [API de Usuários](../guias-api/api-user.md)

---

### Grupos (11 endpoints)

- `GET /group/list` - Listar grupos
- `POST /group/create` - Criar grupo
- `POST /group/info` - Informações do grupo
- `POST /group/participant` - Gerenciar participantes
- `POST /group/photo` - Atualizar foto
- `POST /group/name` - Atualizar nome
- `POST /group/description` - Atualizar descrição
- `POST /group/invitelink` - Gerar link de convite
- `POST /group/join` - Entrar via link
- `POST /group/leave` - Sair do grupo
- `GET /group/myall` - Grupos do usuário

**Documentação completa:** [API de Grupos](../guias-api/api-groups.md)

---

### Chats (7 endpoints)

- `POST /chat/pin` - Fixar conversa
- `POST /chat/unpin` - Desfixar conversa
- `POST /chat/archive` - Arquivar
- `POST /chat/unarchive` - Desarquivar
- `POST /chat/mute` - Silenciar
- `POST /chat/unmute` - Reativar notificações
- `POST /chat/history-sync` - Sincronizar histórico

**Documentação completa:** [API de Chats](../guias-api/api-chats.md)

---

### Labels (6 endpoints)

- `POST /label/chat` - Adicionar label ao chat
- `POST /label/message` - Adicionar label à mensagem
- `POST /label/edit` - Editar label
- `GET /label/list` - Listar labels
- `POST /unlabel/chat` - Remover label do chat
- `POST /unlabel/message` - Remover label da mensagem

**Documentação completa:** [API de Labels](../guias-api/api-labels.md)

---

### Comunidades (3 endpoints)

- `POST /community/create` - Criar comunidade
- `POST /community/add` - Adicionar grupo
- `POST /community/remove` - Remover grupo

**Documentação completa:** [API de Comunidades](../guias-api/api-community.md)

---

### Newsletters (6 endpoints)

- `POST /newsletter/create` - Criar newsletter/canal
- `GET /newsletter/list` - Listar newsletters
- `POST /newsletter/info` - Informações da newsletter
- `POST /newsletter/link` - Gerar link de convite
- `POST /newsletter/subscribe` - Inscrever-se
- `POST /newsletter/messages` - Listar mensagens

**Documentação completa:** [API de Newsletters](../guias-api/api-newsletter.md)

---

### Chamadas (1 endpoint)

- `POST /call/reject` - Rejeitar chamada recebida

**Documentação completa:** [API de Chamadas](../guias-api/api-call.md)

---

## Recursos Adicionais

### Documentação Interativa

- **Swagger UI**: http://localhost:4000/swagger/index.html (quando servidor estiver rodando)
- Teste endpoints diretamente no navegador
- Visualize payloads de request/response
- Documentação completa com exemplos

### Guias de API

- [Visão Geral da API](../guias-api/api-overview.md)
- [Autenticação](../conceitos-core/authentication.md)
- [Sistema de Eventos](../recursos-avancados/events-system.md)

---

**Total de endpoints disponíveis: 80**

**Documentação Evolution GO v1.0**
