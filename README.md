# Bot WhatsApp - Contador Regressivo Diário

Bot em Go que atualiza automaticamente o nome de um grupo do WhatsApp com uma contagem regressiva diária.

## Funcionalidades

- 🔐 Autenticação via QR Code no terminal
- 📅 Atualização automática do nome do grupo na virada de cada dia (00:00)
- 🌍 Usa horário de São Paulo (America/Sao_Paulo)
- 🎯 Contagem regressiva até 01/05/2026 (horário de São Paulo)
- 🧭 Seletor de grupo no terminal (com salvamento automático do ID)
- 💾 Armazenamento local de sessões (não precisa escanear QR code toda vez)

## Requisitos

- Go 1.23 ou superior

## Como usar

1. Instale as dependências:
```bash
go mod download
```

2. **Execute o bot:**
```bash
go run main.go
```

O bot atualiza na **virada do dia** em São Paulo (00:00).  
Também faz uma atualização imediata ao iniciar.

3. Na primeira execução, um QR Code aparecerá no terminal. Escaneie com seu WhatsApp:
   - Abra o WhatsApp no celular
   - Vá em Menu (⋮) > Aparelhos conectados
   - Toque em "Conectar um aparelho"
   - Escaneie o QR Code exibido no terminal

4. Na primeira execução conectada, ele mostrará os grupos disponíveis:
   - Digite o número do grupo desejado
   - O ID fica salvo em `sessions/group_id.txt`
   - Nas próximas execuções, você pode reaproveitar o grupo salvo ou trocar

5. O bot começará a atualizar o nome do grupo automaticamente todos os dias (horário de São Paulo)!

## Configuração

Para alterar a data alvo ou o timezone, edite as constantes no arquivo `main.go`:

```go
const (
    TARGET_YEAR  = 2026            // Ano alvo
    TARGET_MONTH = time.May         // Mês alvo
    TARGET_DAY   = 1                // Dia alvo
    TIMEZONE = "America/Sao_Paulo"  // Timezone de São Paulo
)
```

O ID do grupo é salvo automaticamente em:

```txt
sessions/group_id.txt
```

## Formato do nome do grupo

O bot atualiza o nome do grupo no formato:
```
Faltam X dias para esse tibas
```

No dia do evento (quando faltar 0 dias), ele usa:

```
Hoje é o grande dia 🎉🏖️
```

## Sessões

As sessões são armazenadas na pasta `sessions/` e persistem entre execuções. Para desconectar completamente, delete esta pasta.

## Monitoramento

O bot agora inclui:
- **📅 Atualização diária:** Sempre atualiza na virada do dia (00:00) de São Paulo
- **💓 Heartbeat:** Mostra status a cada 30 minutos com contador regressivo até a próxima virada
- **🔄 Reconexão automática:** Se perder conexão, tenta reconectar
- **📊 Logs detalhados:** Mostra quando cada atualização acontece

### Logs que você verá:

```
🌍 Usando timezone: America/Sao_Paulo
✓ Conectado ao WhatsApp!

🕐 Horário atual (São Paulo): 05/04/2026 15:30:45
⏰ Bot iniciado! Próxima atualização diária às 06/04/2026 00:00 (em 8h29m15s)
Pressione Ctrl+C para sair

[16:00:45] 💓 Bot ativo - próxima atualização diária às 06/04 00:00 (em 7h59m15s)
[16:30:45] 💓 Bot ativo - próxima atualização diária às 06/04 00:00 (em 7h29m15s)

[06/04/2026 00:00:00] 📅 VIRADA DO DIA - atualizando grupo...
[06/04/2026 00:00:00] ✓ Nome do grupo atualizado: Faltam 25 dias para esse tibas
Próxima atualização diária às 07/04/2026 00:00
```

## Troubleshooting

**O bot não está atualizando automaticamente?**
1. Verifique se o bot está rodando e veja os logs de heartbeat (💓)
2. O heartbeat mostra quanto tempo falta até a próxima atualização diária
3. Aguarde até a virada do dia (00:00) no horário de São Paulo
4. Você verá "📅 VIRADA DO DIA" quando atualizar

**Como funciona a atualização diária?**
- O bot usa o **horário de São Paulo** (America/Sao_Paulo)
- Calcula automaticamente quanto tempo falta até a próxima virada do dia
- Não importa quando você inicia, ele sempre agenda para 00:00 de São Paulo
- Depois disso, atualiza diariamente em cada virada de dia

**Importante:**
- Todos os horários são baseados no timezone de São Paulo
- A data alvo (01/05/2026) também é no horário de São Paulo
- Se você estiver em outro timezone, o bot vai considerar a hora de São Paulo

# testemudarnomegrupo
