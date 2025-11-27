package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/mdp/qrterminal/v3"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	waLog "go.mau.fi/whatsmeow/util/log"
)

const (
	// ID do grupo que será atualizado
	GROUP_ID = "120363421307070094@g.us"
	
	// Data alvo: 29/novembro/2025 às 14:00 (horário de São Paulo)
	TARGET_YEAR  = 2025
	TARGET_MONTH = time.November
	TARGET_DAY   = 29
	TARGET_HOUR  = 14
	
	// Timezone de São Paulo
	TIMEZONE = "America/Sao_Paulo"
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

	// Se não estiver logado, mostrar QR code
	if client.Store.ID == nil {
		qrChan, _ := client.GetQRChannel(context.Background())
		err = client.Connect()
		if err != nil {
			panic(err)
		}

		fmt.Println("Escaneie o QR code abaixo com seu WhatsApp:")
		for evt := range qrChan {
			if evt.Event == "code" {
				qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
			} else {
				fmt.Println("Evento de login:", evt.Event)
			}
		}
	} else {
		// Já está logado, apenas conectar
		err = client.Connect()
		if err != nil {
			panic(err)
		}
	}

	fmt.Println("✓ Conectado ao WhatsApp!")

	// Aguardar um pouco para garantir que está tudo sincronizado
	time.Sleep(2 * time.Second)

	// Atualizar o nome do grupo imediatamente
	updateGroupName(client, location)

	// Calcular quanto tempo falta até a próxima hora cheia (horário de São Paulo)
	now := time.Now().In(location)
	nextHour := now.Truncate(time.Hour).Add(time.Hour)
	durationUntilNextHour := nextHour.Sub(now)

	fmt.Printf("\n🕐 Horário atual (São Paulo): %s\n", now.Format("02/01/2006 15:04:05"))
	fmt.Printf("⏰ Bot iniciado! Próxima atualização às %s (em %v)\n", 
		nextHour.Format("15:04"), 
		durationUntilNextHour.Round(time.Second))
	fmt.Printf("Pressione Ctrl+C para sair\n\n")

	// Criar timer para a próxima hora cheia
	firstTimer := time.NewTimer(durationUntilNextHour)

	// Criar ticker de heartbeat para verificar se está funcionando (a cada 5 minutos)
	heartbeatTicker := time.NewTicker(5 * time.Minute)
	defer heartbeatTicker.Stop()

	// Capturar sinais de interrupção
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	firstTimerFired := false
	var ticker *time.Ticker

	// Loop principal
	for {
		select {
		case t := <-firstTimer.C:
			// Primeira atualização na hora cheia
			spTime := t.In(location)
			fmt.Printf("\n[%s] ⏰ HORA CHEIA ATINGIDA - atualizando grupo...\n", spTime.Format("02/01/2006 15:04:05"))
			updateGroupName(client, location)
			nextUpdate := time.Now().In(location).Add(1 * time.Hour)
			fmt.Printf("Próxima atualização às %s\n\n", nextUpdate.Format("15:04"))
			
			// Criar ticker para as próximas atualizações horárias
			ticker = time.NewTicker(1 * time.Hour)
			defer ticker.Stop()
			firstTimerFired = true
			
		case t := <-func() <-chan time.Time {
			if ticker != nil {
				return ticker.C
			}
			// Retornar canal que nunca envia nada
			return make(<-chan time.Time)
		}():
			spTime := t.In(location)
			fmt.Printf("\n[%s] ⏰ HORA CHEIA ATINGIDA - atualizando grupo...\n", spTime.Format("02/01/2006 15:04:05"))
			updateGroupName(client, location)
			nextUpdate := time.Now().In(location).Add(1 * time.Hour)
			fmt.Printf("Próxima atualização às %s\n\n", nextUpdate.Format("15:04"))
			
		case t := <-heartbeatTicker.C:
			spTime := t.In(location)
			if !firstTimerFired {
				nowSP := time.Now().In(location)
				nextHourTime := nowSP.Truncate(time.Hour).Add(time.Hour)
				timeUntil := nextHourTime.Sub(nowSP).Round(time.Second)
				fmt.Printf("[%s] 💓 Bot ativo - próxima atualização às %s (em %v)\n", 
					spTime.Format("15:04:05"),
					nextHourTime.Format("15:04"),
					timeUntil)
			} else {
				nowSP := time.Now().In(location)
				nextHourTime := nowSP.Truncate(time.Hour).Add(time.Hour)
				fmt.Printf("[%s] 💓 Bot ativo - próxima atualização às %s\n", 
					spTime.Format("15:04:05"),
					nextHourTime.Format("15:04"))
			}
			
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

// Função para calcular horas restantes até a data alvo
func calculateRemainingHours(location *time.Location) int {
	now := time.Now().In(location)
	target := time.Date(TARGET_YEAR, TARGET_MONTH, TARGET_DAY, TARGET_HOUR, 0, 0, 0, location)
	
	diff := target.Sub(now)
	hours := int(diff.Hours())
	
	// Se já passou da data, retornar 0
	if hours < 0 {
		return 0
	}
	
	return hours
}

// Função para atualizar o nome do grupo
func updateGroupName(client *whatsmeow.Client, location *time.Location) {
	hoursRemaining := calculateRemainingHours(location)
	
	// Formato: "Vamo lá dia 29 emm (faltam Xhoras)"
	newName := fmt.Sprintf("Vamo lá dia 29 emm (faltam %dhoras)", hoursRemaining)
	
	// Parse do JID do grupo
	groupJID, err := types.ParseJID(GROUP_ID)
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
	
	// Se chegou a 0 horas, mostrar mensagem especial
	if hoursRemaining == 0 {
		fmt.Println("🎉 COMEÇOU COMEÇOU COMEÇOU!")
	}
}

