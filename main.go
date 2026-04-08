package main

import (
	"bufio"
	"context"
	"fmt"
	"math"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/mdp/qrterminal/v3"
	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

const (
	// Data alvo: 01/maio/2026 (horário de São Paulo)
	TARGET_YEAR  = 2026
	TARGET_MONTH = time.May
	TARGET_DAY   = 1

	// Timezone de São Paulo
	TIMEZONE = "America/Sao_Paulo"

	// Arquivo para persistir o ID do grupo selecionado
	GROUP_ID_FILE = "./sessions/group_id.txt"
)

type pendingCreateCota struct {
	StartedAt time.Time
}

type groupCommandBot struct {
	client      *whatsmeow.Client
	targetGroup types.JID

	mu          sync.Mutex
	pendingCota map[string]pendingCreateCota
}

func newGroupCommandBot(client *whatsmeow.Client, targetGroup types.JID) *groupCommandBot {
	return &groupCommandBot{
		client:      client,
		targetGroup: targetGroup,
		pendingCota: make(map[string]pendingCreateCota),
	}
}

func (b *groupCommandBot) HandleEvent(evt interface{}) {
	switch v := evt.(type) {
	case *events.Message:
		b.handleMessage(v)
	}
}

func (b *groupCommandBot) handleMessage(evt *events.Message) {
	if evt == nil || evt.Message == nil {
		return
	}
	if !evt.Info.IsGroup {
		return
	}
	if evt.Info.Chat.String() != b.targetGroup.String() {
		return
	}

	text := extractIncomingText(evt.Message)
	if text == "" {
		return
	}

	messageBody, hasBraces := extractBracedContent(text)
	senderKey := buildSenderKey(evt.Info.Chat, evt.Info.Sender)

	if b.isAwaitingCreateCota(senderKey) {
		if !hasBraces {
			return
		}

		command := normalizeCommand(messageBody)
		if command == "criar cota" {
			b.markAwaitingCreateCota(senderKey)
			b.sendText(evt.Info.Chat, buildCreateCotaPrompt())
			return
		}

		formattedMessage, err := buildCreateCotaMessage(messageBody)
		if err != nil {
			b.sendText(evt.Info.Chat, fmt.Sprintf("❌ %v\n\nUse o formato:\n{titulo, valor total, quantidade de pessoas, pix, nome do pix}", err))
			return
		}

		b.clearAwaitingCreateCota(senderKey)
		b.sendText(evt.Info.Chat, formattedMessage)
		return
	}

	if !hasBraces {
		return
	}

	switch normalizeCommand(messageBody) {
	case "criar cota":
		b.markAwaitingCreateCota(senderKey)
		b.sendText(evt.Info.Chat, buildCreateCotaPrompt())
	default:
		b.sendText(evt.Info.Chat, "Comando não reconhecido. Use {criar cota}.")
	}
}

func (b *groupCommandBot) sendText(to types.JID, text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	_, err := b.client.SendMessage(context.Background(), to, &waProto.Message{
		Conversation: proto.String(text),
	})
	if err != nil {
		fmt.Printf("❌ Erro ao enviar mensagem no grupo %s: %v\n", to.String(), err)
	}
}

func (b *groupCommandBot) isAwaitingCreateCota(senderKey string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	_, exists := b.pendingCota[senderKey]
	return exists
}

func (b *groupCommandBot) markAwaitingCreateCota(senderKey string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.pendingCota[senderKey] = pendingCreateCota{StartedAt: time.Now()}
}

func (b *groupCommandBot) clearAwaitingCreateCota(senderKey string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.pendingCota, senderKey)
}

func buildSenderKey(chatJID types.JID, senderJID types.JID) string {
	return chatJID.String() + "|" + senderJID.String()
}

func extractIncomingText(msg *waProto.Message) string {
	if msg == nil {
		return ""
	}

	if text := strings.TrimSpace(msg.GetConversation()); text != "" {
		return text
	}

	if text := strings.TrimSpace(msg.GetExtendedTextMessage().GetText()); text != "" {
		return text
	}

	return ""
}

func extractBracedContent(text string) (string, bool) {
	text = strings.TrimSpace(text)
	if len(text) < 3 {
		return "", false
	}
	if !strings.HasPrefix(text, "{") || !strings.HasSuffix(text, "}") {
		return "", false
	}

	content := strings.TrimSpace(text[1 : len(text)-1])
	if content == "" {
		return "", false
	}
	return content, true
}

func normalizeCommand(command string) string {
	command = strings.TrimSpace(strings.ToLower(command))
	command = strings.Join(strings.Fields(command), " ")
	return command
}

func buildCreateCotaPrompt() string {
	return "Me envie os dados respondendo nesse formato:\n{titulo, valor total, quantidade de pessoas, pix, nome do pix}\n\nExemplo:\n{LISTA DE PAGAMENTO DA VAN ATÉ 13/04, 2000, 15, 84996465312, Lucas Matheus Alexandre da Silva}"
}

func buildCreateCotaMessage(payload string) (string, error) {
	parts := splitAndTrim(payload, ",")
	if len(parts) != 5 {
		return "", fmt.Errorf("formato inválido. São 5 campos separados por vírgula")
	}

	title := parts[0]
	totalRaw := parts[1]
	peopleRaw := parts[2]
	pix := parts[3]
	pixOwner := parts[4]

	if title == "" || totalRaw == "" || peopleRaw == "" || pix == "" || pixOwner == "" {
		return "", fmt.Errorf("todos os campos são obrigatórios")
	}

	total, err := parseBrazilianMoney(totalRaw)
	if err != nil {
		return "", fmt.Errorf("valor total inválido")
	}
	if total <= 0 {
		return "", fmt.Errorf("o valor total precisa ser maior que zero")
	}

	people, err := strconv.Atoi(strings.TrimSpace(peopleRaw))
	if err != nil {
		return "", fmt.Errorf("quantidade de pessoas inválida")
	}
	if people < 1 {
		return "", fmt.Errorf("a quantidade de pessoas precisa ser maior que zero")
	}
	if people > 300 {
		return "", fmt.Errorf("quantidade de pessoas muito alta (máximo: 300)")
	}

	return buildPaymentListMessage(title, total, people, pix, pixOwner), nil
}

func splitAndTrim(input string, sep string) []string {
	raw := strings.Split(input, sep)
	parts := make([]string, 0, len(raw))
	for _, part := range raw {
		parts = append(parts, strings.TrimSpace(part))
	}
	return parts
}

func parseBrazilianMoney(raw string) (float64, error) {
	sanitized := strings.TrimSpace(raw)
	sanitized = strings.ReplaceAll(sanitized, "R$", "")
	sanitized = strings.ReplaceAll(sanitized, "r$", "")
	sanitized = strings.ReplaceAll(sanitized, " ", "")

	hasDot := strings.Contains(sanitized, ".")
	hasComma := strings.Contains(sanitized, ",")

	switch {
	case hasDot && hasComma:
		sanitized = strings.ReplaceAll(sanitized, ".", "")
		sanitized = strings.ReplaceAll(sanitized, ",", ".")
	case hasComma:
		sanitized = strings.ReplaceAll(sanitized, ",", ".")
	case strings.Count(sanitized, ".") > 1:
		sanitized = strings.ReplaceAll(sanitized, ".", "")
	}

	var cleaned strings.Builder
	for i, r := range sanitized {
		if r >= '0' && r <= '9' {
			cleaned.WriteRune(r)
			continue
		}
		if r == '.' {
			cleaned.WriteRune(r)
			continue
		}
		if r == '-' && i == 0 {
			cleaned.WriteRune(r)
		}
	}

	parsed := cleaned.String()
	if parsed == "" || parsed == "." || parsed == "-" {
		return 0, fmt.Errorf("valor inválido")
	}

	value, err := strconv.ParseFloat(parsed, 64)
	if err != nil {
		return 0, err
	}

	return math.Round(value*100) / 100, nil
}

func buildPaymentListMessage(title string, total float64, people int, pix string, pixOwner string) string {
	perPerson := total / float64(people)

	var out strings.Builder
	out.WriteString(fmt.Sprintf("%s - R$%s\n\n", strings.ToUpper(strings.TrimSpace(title)), formatBrazilianNumber(total)))
	out.WriteString(fmt.Sprintf("%s (por pessoa)\n\n", formatBrazilianNumber(perPerson)))
	out.WriteString(fmt.Sprintf("Pix: %s\n\n", strings.TrimSpace(pix)))
	out.WriteString(strings.TrimSpace(pixOwner))
	out.WriteString("\n\n")

	for i := 1; i <= people; i++ {
		out.WriteString(fmt.Sprintf("%d-\n", i))
	}

	return strings.TrimRight(out.String(), "\n")
}

func formatBrazilianNumber(value float64) string {
	rounded := math.Round(value*100) / 100
	isNegative := rounded < 0
	if isNegative {
		rounded = -rounded
	}

	decimal := fmt.Sprintf("%.2f", rounded)
	parts := strings.SplitN(decimal, ".", 2)

	intPart := parts[0]
	decimalPart := "00"
	if len(parts) == 2 {
		decimalPart = parts[1]
	}

	intPart = addThousandsSeparator(intPart, '.')

	if isNegative {
		return "-" + intPart + "," + decimalPart
	}
	return intPart + "," + decimalPart
}

func addThousandsSeparator(number string, separator rune) string {
	if len(number) <= 3 {
		return number
	}

	firstGroupLen := len(number) % 3
	if firstGroupLen == 0 {
		firstGroupLen = 3
	}

	var formatted strings.Builder
	formatted.WriteString(number[:firstGroupLen])

	for i := firstGroupLen; i < len(number); i += 3 {
		formatted.WriteRune(separator)
		formatted.WriteString(number[i : i+3])
	}

	return formatted.String()
}

func main() {
	// Criar pasta para armazenar sessões
	os.MkdirAll("./sessions", 0755)

	// Carregar timezone de São Paulo
	location, err := time.LoadLocation(TIMEZONE)
	if err != nil {
		fmt.Printf("⚠️  Erro ao carregar timezone %s: %v\n", TIMEZONE, err)
		fmt.Println("Usando horário local do sistema...")
		location = time.Local
	} else {
		fmt.Printf("🌍 Usando timezone: %s\n", TIMEZONE)
	}

	// Configurar logger
	logger := waLog.Stdout("Main", "INFO", true)

	// Tentar sincronizar a versão Web do WhatsApp para evitar erro de client outdated
	syncLatestWAVersion()

	// Criar contexto
	ctx := context.Background()

	// Configurar banco de dados para armazenar sessões
	container, err := sqlstore.New(ctx, "sqlite3", "file:./sessions/whatsapp.db?_foreign_keys=on", logger)
	if err != nil {
		panic(err)
	}

	// Pegar o primeiro dispositivo disponível ou criar um novo
	deviceStore, err := container.GetFirstDevice(ctx)
	if err != nil {
		panic(err)
	}

	// Criar cliente WhatsApp
	client := whatsmeow.NewClient(deviceStore, logger)

	// Conectar e, se necessário, parear via QR code
	err = connectClient(client)
	if err != nil {
		fmt.Printf("❌ Falha ao conectar no WhatsApp: %v\n", err)
		client.Disconnect()
		return
	}

	fmt.Println("✓ Conectado ao WhatsApp!")

	// Aguardar um pouco para garantir que está tudo sincronizado
	time.Sleep(2 * time.Second)

	// Selecionar grupo no terminal e salvar o ID
	groupID, err := selectGroupID(client)
	if err != nil {
		fmt.Printf("❌ Erro ao selecionar grupo: %v\n", err)
		client.Disconnect()
		return
	}
	fmt.Printf("📌 Grupo selecionado: %s\n", groupID)

	groupJID, err := types.ParseJID(groupID)
	if err != nil {
		fmt.Printf("❌ ID de grupo inválido: %v\n", err)
		client.Disconnect()
		return
	}

	commandBot := newGroupCommandBot(client, groupJID)
	client.AddEventHandler(commandBot.HandleEvent)
	fmt.Println("🤖 Comandos ativos no grupo selecionado (use {criar cota})")

	// Atualizar o nome do grupo imediatamente
	updateGroupName(client, location, groupID)

	// Calcular quanto tempo falta até a próxima virada do dia (horário de São Paulo)
	now := time.Now().In(location)
	nextDailyUpdate := getNextDailyUpdate(location)
	durationUntilNextUpdate := nextDailyUpdate.Sub(now)

	fmt.Printf("\n🕐 Horário atual (São Paulo): %s\n", now.Format("02/01/2006 15:04:05"))
	fmt.Printf("⏰ Bot iniciado! Próxima atualização diária às %s (em %v)\n",
		nextDailyUpdate.Format("02/01/2006 15:04"),
		durationUntilNextUpdate.Round(time.Second))
	fmt.Printf("Pressione Ctrl+C para sair\n\n")

	// Criar timer para a próxima virada do dia
	dailyTimer := time.NewTimer(durationUntilNextUpdate)
	defer dailyTimer.Stop()

	// Criar ticker de heartbeat para verificar se está funcionando (a cada 30 minutos)
	heartbeatTicker := time.NewTicker(30 * time.Minute)
	defer heartbeatTicker.Stop()

	// Capturar sinais de interrupção
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	// Loop principal
	for {
		select {
		case t := <-dailyTimer.C:
			spTime := t.In(location)
			fmt.Printf("\n[%s] 📅 VIRADA DO DIA - atualizando grupo...\n", spTime.Format("02/01/2006 15:04:05"))
			updateGroupName(client, location, groupID)
			nextDailyUpdate = getNextDailyUpdate(location)
			durationUntilNextUpdate = nextDailyUpdate.Sub(time.Now().In(location))
			fmt.Printf("Próxima atualização diária às %s\n\n", nextDailyUpdate.Format("02/01/2006 15:04"))
			dailyTimer.Reset(durationUntilNextUpdate)

		case t := <-heartbeatTicker.C:
			spTime := t.In(location)
			nowSP := time.Now().In(location)
			nextDaily := getNextDailyUpdate(location)
			timeUntil := nextDaily.Sub(nowSP).Round(time.Second)
			fmt.Printf("[%s] 💓 Bot ativo - próxima atualização diária às %s (em %v)\n",
				spTime.Format("15:04:05"),
				nextDaily.Format("02/01 15:04"),
				timeUntil)

			// Verificar se o cliente ainda está conectado
			if !client.IsConnected() {
				fmt.Println("⚠️  Conexão perdida, reconectando...")
				err := client.Connect()
				if err != nil {
					fmt.Printf("❌ Erro ao reconectar: %v\n", err)
				} else {
					fmt.Println("✓ Reconectado com sucesso!")
				}
			}

		case <-c:
			fmt.Println("\nDesconectando...")
			client.Disconnect()
			return
		}
	}
}

func syncLatestWAVersion() {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()

	httpClient := &http.Client{Timeout: 10 * time.Second}
	latest, err := whatsmeow.GetLatestVersion(ctx, httpClient)
	if err != nil {
		fmt.Printf("⚠️  Não foi possível buscar versão mais recente do WhatsApp Web: %v\n", err)
		fmt.Printf("Continuando com versão embutida: %s\n", store.GetWAVersion().String())
		return
	}

	current := store.GetWAVersion()
	if current.LessThan(*latest) {
		store.SetWAVersion(*latest)
		fmt.Printf("⬆️  Versão WA Web atualizada em runtime: %s -> %s\n", current.String(), latest.String())
	} else {
		fmt.Printf("✅ Versão WA Web em uso: %s\n", current.String())
	}
}

func connectClient(client *whatsmeow.Client) error {
	if client.Store.ID != nil {
		if err := client.Connect(); err != nil {
			return err
		}
		if !waitForConnectionWithRetry(client, 2, 20*time.Second) {
			return fmt.Errorf("timeout ao estabelecer conexão websocket")
		}
		return nil
	}

	qrChan, err := client.GetQRChannel(context.Background())
	if err != nil {
		return fmt.Errorf("erro ao criar canal de QR code: %w", err)
	}

	if err := client.Connect(); err != nil {
		return err
	}

	fmt.Println("Escaneie o QR code abaixo com seu WhatsApp:")
	for evt := range qrChan {
		switch evt.Event {
		case whatsmeow.QRChannelEventCode:
			qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
		case "success":
			// Em alguns ambientes o pareamento conclui e o websocket cai logo em seguida.
			// Nesses casos, tentamos reconectar automaticamente antes de falhar.
			if !waitForConnectionWithRetry(client, 3, 20*time.Second) {
				return fmt.Errorf("pareamento concluído, mas websocket não conectou")
			}
			return nil
		case "timeout":
			return fmt.Errorf("timeout ao aguardar leitura do QR code")
		case "err-client-outdated":
			return fmt.Errorf("cliente desatualizado (err-client-outdated). Atualize a dependência go.mau.fi/whatsmeow e tente novamente")
		case whatsmeow.QRChannelEventError:
			if evt.Error != nil {
				return fmt.Errorf("erro de pareamento: %w", evt.Error)
			}
			return fmt.Errorf("erro de pareamento")
		default:
			fmt.Println("Evento de login:", evt.Event)
		}
	}

	if client.IsConnected() {
		return nil
	}
	return fmt.Errorf("canal de pareamento foi encerrado sem conexão")
}

func waitForConnectionWithRetry(client *whatsmeow.Client, attempts int, timeout time.Duration) bool {
	if attempts < 1 {
		attempts = 1
	}
	for i := 1; i <= attempts; i++ {
		if client.IsConnected() || client.WaitForConnection(timeout) {
			return true
		}
		if i == attempts {
			break
		}
		fmt.Printf("⚠️  Conexão não estabilizou após pareamento. Tentando reconectar (%d/%d)...\n", i+1, attempts)
		if err := client.Connect(); err != nil {
			fmt.Printf("⚠️  Reconexão falhou: %v\n", err)
		}
		time.Sleep(2 * time.Second)
	}
	return client.IsConnected()
}

// Função para calcular dias restantes até a data alvo (baseado em data, não hora)
func calculateRemainingDays(location *time.Location) int {
	now := time.Now().In(location)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, location)
	target := time.Date(TARGET_YEAR, TARGET_MONTH, TARGET_DAY, 0, 0, 0, 0, location)

	diff := target.Sub(today)
	days := int(diff.Hours() / 24)

	// Se já passou da data, retornar 0 para manter o título especial
	if days < 0 {
		return 0
	}

	return days
}

func getNextDailyUpdate(location *time.Location) time.Time {
	now := time.Now().In(location)
	return time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, location)
}

func buildGroupName(location *time.Location) string {
	daysRemaining := calculateRemainingDays(location)
	if daysRemaining == 0 {
		return "Hoje é o grande dia 🎉🏖️"
	}
	return fmt.Sprintf("Faltam %d dias para esse tibas", daysRemaining)
}

func loadSavedGroupID() string {
	data, err := os.ReadFile(GROUP_ID_FILE)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func saveGroupID(groupID string) error {
	return os.WriteFile(GROUP_ID_FILE, []byte(strings.TrimSpace(groupID)), 0644)
}

func selectGroupID(client *whatsmeow.Client) (string, error) {
	savedGroupID := loadSavedGroupID()
	groups, err := getJoinedGroupsWithRetry(client, 3)
	if err != nil {
		if savedGroupID != "" {
			fmt.Printf("⚠️  Não foi possível listar grupos agora. Usando ID salvo: %s\n", savedGroupID)
			return savedGroupID, nil
		}
		return "", fmt.Errorf("erro ao listar grupos: %w", err)
	}

	if len(groups) == 0 {
		if savedGroupID != "" {
			fmt.Printf("⚠️  Nenhum grupo retornado. Usando ID salvo: %s\n", savedGroupID)
			return savedGroupID, nil
		}
		return "", fmt.Errorf("nenhum grupo encontrado para esta conta")
	}

	reader := bufio.NewReader(os.Stdin)
	savedIndex := -1
	for i, g := range groups {
		if g.JID.String() == savedGroupID {
			savedIndex = i
			break
		}
	}

	if savedIndex >= 0 {
		fmt.Printf("\n📌 Grupo salvo atual: %s | %s\n", groups[savedIndex].Name, savedGroupID)
		fmt.Print("Pressione Enter para usar esse grupo, ou digite 'trocar' para escolher outro: ")
		input, _ := reader.ReadString('\n')
		if strings.TrimSpace(strings.ToLower(input)) != "trocar" {
			return savedGroupID, nil
		}
	}

	fmt.Println("\n📋 Grupos disponíveis:")
	for i, g := range groups {
		fmt.Printf("[%d] %s | %s\n", i+1, g.Name, g.JID.String())
	}

	for {
		fmt.Print("\nDigite o número do grupo que deseja usar: ")
		input, err := reader.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("erro ao ler seleção: %w", err)
		}

		input = strings.TrimSpace(input)
		index, err := strconv.Atoi(input)
		if err != nil || index < 1 || index > len(groups) {
			fmt.Println("❌ Opção inválida. Digite um número da lista.")
			continue
		}

		selected := groups[index-1].JID.String()
		if err := saveGroupID(selected); err != nil {
			fmt.Printf("⚠️  Não foi possível salvar o grupo selecionado: %v\n", err)
		} else {
			fmt.Printf("✓ Grupo salvo em %s\n", GROUP_ID_FILE)
		}
		return selected, nil
	}
}

func getJoinedGroupsWithRetry(client *whatsmeow.Client, attempts int) ([]*types.GroupInfo, error) {
	if attempts < 1 {
		attempts = 1
	}
	ctx := context.Background()
	var lastErr error

	for i := 1; i <= attempts; i++ {
		if !client.IsConnected() {
			fmt.Printf("⚠️  Cliente desconectado ao listar grupos. Reconectando (%d/%d)...\n", i, attempts)
			if err := client.Connect(); err != nil {
				lastErr = err
				time.Sleep(2 * time.Second)
				continue
			}
			if !waitForConnectionWithRetry(client, 2, 15*time.Second) {
				lastErr = fmt.Errorf("não conectou após reconexão")
				time.Sleep(2 * time.Second)
				continue
			}
		}

		groups, err := client.GetJoinedGroups(ctx)
		if err == nil {
			return groups, nil
		}
		lastErr = err
		time.Sleep(2 * time.Second)
	}

	return nil, lastErr
}

// Função para atualizar o nome do grupo
func updateGroupName(client *whatsmeow.Client, location *time.Location, groupID string) {
	newName := buildGroupName(location)

	// Parse do JID do grupo
	groupJID, err := types.ParseJID(groupID)
	if err != nil {
		fmt.Printf("❌ Erro ao fazer parse do ID do grupo: %v\n", err)
		return
	}

	// Verificar se está conectado
	if !client.IsConnected() {
		fmt.Println("⚠️  Cliente não está conectado! Tentando reconectar...")
		err := client.Connect()
		if err != nil {
			fmt.Printf("❌ Erro ao reconectar: %v\n", err)
			return
		}
		time.Sleep(2 * time.Second)
	}

	// Atualizar o nome do grupo
	ctx := context.Background()
	err = client.SetGroupName(ctx, groupJID, newName)
	if err != nil {
		fmt.Printf("❌ Erro ao atualizar nome do grupo: %v\n", err)
		return
	}

	timestamp := time.Now().In(location).Format("02/01/2006 15:04:05")
	fmt.Printf("[%s] ✓ Nome do grupo atualizado: %s\n", timestamp, newName)
}
