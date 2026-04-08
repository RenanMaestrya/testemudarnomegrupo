package main

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/mdp/qrterminal/v3"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	waLog "go.mau.fi/whatsmeow/util/log"
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
		if !client.WaitForConnection(20 * time.Second) {
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
			if !client.WaitForConnection(20 * time.Second) {
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
	ctx := context.Background()
	groups, err := client.GetJoinedGroups(ctx)
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
