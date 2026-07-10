package llm

import (
	"context"
	"fmt"
	"strings"
	"time"

	"patcode/tools"
)

type BuiltinProvider struct {
	modelName string
}

func NewBuiltinProvider() *BuiltinProvider {
	return &BuiltinProvider{modelName: "patcode-builtin"}
}

func (p *BuiltinProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	return nil, fmt.Errorf("use ChatStream")
}

func (p *BuiltinProvider) ChatStream(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error) {
	ch := make(chan StreamEvent, 32)

	go func() {
		defer close(ch)

		lastMsg := ""
		if len(req.Messages) > 0 {
			lastMsg = req.Messages[len(req.Messages)-1].Content
		}

		response := p.generateResponse(lastMsg, req.System, req.Tools)

		words := strings.Fields(response)
		for i, w := range words {
			select {
			case <-ctx.Done():
				ch <- StreamEvent{Type: StreamDone, Done: true}
				return
			default:
			}

			ch <- StreamEvent{Type: StreamChunk, Content: w + " "}
			time.Sleep(20 * time.Millisecond)

			if i > 0 && i%10 == 0 {
				time.Sleep(10 * time.Millisecond)
			}
		}
		ch <- StreamEvent{Type: StreamDone, Done: true}
	}()

	return ch, nil
}

func (p *BuiltinProvider) generateResponse(userMsg, system string, tools []tools.ToolDefinition) string {
	msg := strings.ToLower(userMsg)

	if strings.Contains(msg, "hello") || strings.Contains(msg, "hi") || strings.Contains(msg, "cześć") || strings.Contains(msg, "hej") {
		return "Cześć! Jestem patcode - twój lokalny asystent kodowania. W czym mogę pomóc?"
	}

	if strings.Contains(msg, "read") || strings.Contains(msg, "przeczytaj") || strings.Contains(msg, "pokaż") {
		return "Użyj narzędzia `read` żeby przeczytać plik. Np: `/read main.go`"
	}

	if strings.Contains(msg, "write") || strings.Contains(msg, "stworz") || strings.Contains(msg, "utwórz") || strings.Contains(msg, "zapisz") {
		return "Użyj narzędzia `write` żeby stworzyć plik. Np: `/write nowy_plik.go 'package main...'`"
	}

	if strings.Contains(msg, "edit") || strings.Contains(msg, "zmień") || strings.Contains(msg, "popraw") {
		return "Użyj narzędzia `edit` żeby zmienić fragment pliku. Potrzebuję ścieżki, starego tekstu i nowego tekstu."
	}

	if strings.Contains(msg, "bash") || strings.Contains(msg, "terminal") || strings.Contains(msg, "polecenie") || strings.Contains(msg, "uruchom") {
		return "Użyj narzędzia `bash` żeby uruchomić polecenie. Np: `/bash go build`"
	}

	if strings.Contains(msg, "grep") || strings.Contains(msg, "szukaj") || strings.Contains(msg, "znajdź") {
		return "Użyj narzędzia `grep` żeby szukać w kodzie. Np: `/grep 'func main'`"
	}

	if strings.Contains(msg, "glob") || strings.Contains(msg, "pliki") || strings.Contains(msg, "lista") {
		return "Użyj narzędzia `glob` żeby znaleźć pliki. Np: `/glob '**/*.go'`"
	}

	if strings.Contains(msg, "plan") {
		return "Tryb PLAN - tylko planuję, nie zmieniam plików. Przełącz na BUILD (/build) żeby wykonywać zmiany."
	}

	if strings.Contains(msg, "build") {
		return "Tryb BUILD - mogę teraz używać narzędzi do edycji plików."
	}

	if strings.Contains(msg, "test") || strings.Contains(msg, "testy") {
		return "Uruchom testy: `/bash go test ./...`"
	}

	return "Rozumiem. Mogę pomóc z czytaniem, pisaniem, edycją plików, wyszukiwaniem w kodzie i uruchamianiem poleceń. Co chcesz zrobić?"
}
