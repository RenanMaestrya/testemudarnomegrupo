# Bot WhatsApp - Contador Regressivo

Bot em Go que atualiza automaticamente o nome de um grupo do WhatsApp com uma contagem regressiva.

## Funcionalidades

- 🔐 Autenticação via QR Code no terminal
- ⏰ Atualização automática do nome do grupo nas horas cheias (16:00, 17:00, 18:00...)
- 🌍 Usa horário de São Paulo (America/Sao_Paulo)
- 📅 Contagem regressiva até 29/11/2025 às 14:00 (horário de São Paulo)
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

O bot atualiza nas **HORAS CHEIAS** de São Paulo (16:00, 17:00, 18:00...)  
Exemplo: Se você rodar às 15:30, ele vai atualizar às 16:00, depois 17:00, 18:00, etc.

3. Na primeira execução, um QR Code aparecerá no terminal. Escaneie com seu WhatsApp:
   - Abra o WhatsApp no celular
   - Vá em Menu (⋮) > Aparelhos conectados
   - Toque em "Conectar um aparelho"
   - Escaneie o QR Code exibido no terminal

4. O bot começará a atualizar o nome do grupo automaticamente nas horas cheias (horário de São Paulo)!

## Configuração

Para alterar o grupo, a data alvo ou o timezone, edite as constantes no arquivo `main.go`:

```go
const (
    GROUP_ID = "120363421307070094@g.us"  // ID do grupo
    TARGET_YEAR  = 2025                   // Ano alvo
    TARGET_MONTH = time.November          // Mês alvo
    TARGET_DAY   = 29                     // Dia alvo
    TARGET_HOUR  = 14                     // Hora alvo
    TIMEZONE = "America/Sao_Paulo"        // Timezone de São Paulo
)
```

## Formato do nome do grupo

O bot atualiza o nome do grupo no formato:
```
Vamo lá dia 29 emm (faltam Xhoras)
```

Onde X é o número de horas restantes até a data alvo.

## Sessões

As sessões são armazenadas na pasta `sessions/` e persistem entre execuções. Para desconectar completamente, delete esta pasta.

## Monitoramento

O bot agora inclui:
- **⏰ Atualização nas horas cheias:** Não importa quando você inicia, ele sempre atualiza nas horas cheias (ex: 16:00, 17:00, 18:00...)
- **💓 Heartbeat:** Mostra status a cada 5 minutos com contador regressivo
- **🔄 Reconexão automática:** Se perder conexão, tenta reconectar
- **📊 Logs detalhados:** Mostra quando cada atualização acontece

### Logs que você verá:

```
🌍 Usando timezone: America/Sao_Paulo
✓ Conectado ao WhatsApp!

🕐 Horário atual (São Paulo): 27/11/2025 15:30:45
⏰ Bot iniciado! Próxima atualização às 16:00 (em 29m15s)
Pressione Ctrl+C para sair

[15:35:45] 💓 Bot ativo - próxima atualização às 16:00 (em 24m15s)
[15:40:45] 💓 Bot ativo - próxima atualização às 16:00 (em 19m15s)

[27/11/2025 16:00:00] ⏰ HORA CHEIA ATINGIDA - atualizando grupo...
[27/11/2025 16:00:00] ✓ Nome do grupo atualizado: Vamo lá dia 29 emm (faltam 46horas)
Próxima atualização às 17:00
```

## Troubleshooting

**O bot não está atualizando automaticamente?**
1. Verifique se o bot está rodando e veja os logs de heartbeat (💓)
2. O heartbeat mostra quanto tempo falta até a próxima atualização
3. Aguarde até a próxima hora cheia (16:00, 17:00, etc) do horário de São Paulo
4. Você verá "⏰ HORA CHEIA ATINGIDA" quando atualizar

**Como funciona a atualização nas horas cheias?**
- O bot usa o **horário de São Paulo** (America/Sao_Paulo)
- Calcula automaticamente quanto tempo falta até a próxima hora cheia
- Não importa quando você inicia (15:10, 15:30, 15:50...), ele sempre atualiza na hora cheia de São Paulo (16:00)
- Depois disso, atualiza a cada hora cheia: 17:00, 18:00, 19:00, etc.

**Importante:**
- Todos os horários são baseados no timezone de São Paulo
- A data alvo (29/11/2025 14:00) também é no horário de São Paulo
- Se você estiver em outro timezone, o bot vai considerar a hora de São Paulo

# testemudarnomegrupo
