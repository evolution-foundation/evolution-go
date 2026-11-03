# Evolution Go 0.7.2 no Coolify

Este fork aplica ao handshake do whatsmeow a versão do WhatsApp Web detectada
na primeira conexão do processo, com base no
[PR upstream #227](https://github.com/evolution-foundation/evolution-go/pull/227).
A versão fica fixa até o processo reiniciar. A dependência do whatsmeow permanece
na mesma revisão da tag 0.7.2.

1. Crie uma aplicação a partir do repositório deste fork. Selecione a branch
   `feat/0.7.2-whatsapp-version`, use a estratégia **Docker Compose**, diretório
   base `/` e localização `/compose.coolify.yaml`.
2. Configure `GLOBAL_API_KEY` com a chave administrativa da Evolution Go.
   Gere `POSTGRES_PASSWORD` com `openssl rand -hex 32` para usar um valor seguro
   que também seja válido dentro das URLs PostgreSQL.
3. Deixe `WHATSAPP_VERSION_MAJOR`, `WHATSAPP_VERSION_MINOR` e
   `WHATSAPP_VERSION_PATCH` vazias para detecção automática. Para fixar uma
   revisão conhecida, preencha as três e reinicie a aplicação.
4. Em **Domains**, associe o domínio de homologação ao serviço `evolution-go`,
   com a porta interna `8080`. Exemplo `https://evo-teste.seudominio.com:8080`.
   Se for usar pareamento por passkey, configure `PASSKEY_PUBLIC_URL` com a
   URL pública que o navegador acessa, normalmente sem o sufixo `:8080`.
5. Faça o deploy. Se a instalação pedir licença, siga a ativação em `/manager`.
   `EVOLUTION_OPERATOR_EMAIL` é opcional e serve para ativação automática quando
   o e-mail já está cadastrado.
6. Crie uma instância de teste e solicite o QR. Nos logs, procure
   `Applied WhatsApp handshake version`, escaneie o QR e confirme que a
   instância ficou conectada. Se a consulta ao WhatsApp Web falhar na primeira
   conexão, o log mostra a versão usada como fallback. Reinicie o serviço
   para tentar detectar novamente.

O PostgreSQL fica na rede interna do Compose. Os dois bancos são criados
quando o volume do PostgreSQL é inicializado pela primeira vez. O health check
em `/server/ok` indica que a API está respondendo. Valide a licença e o pareamento
no Manager antes de apontar o SaaS para a instalação.
