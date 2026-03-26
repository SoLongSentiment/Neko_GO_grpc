package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	fyneapp "fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/structpb"

	nekogrpc "neko_grpc"
)

type grpcGUI struct {
	app    fyne.App
	window fyne.Window

	addrEntry   *widget.Entry
	tokenEntry  *widget.Entry
	statusLabel *widget.Label

	keyEntry      *widget.Entry
	valueEntry    *widget.Entry
	watchKeyEntry *widget.Entry
	kvOutput      *widget.Entry

	roomEntry    *widget.Entry
	userEntry    *widget.Entry
	messageEntry *widget.Entry
	chatOutput   *widget.Entry

	conn       *grpc.ClientConn
	client     nekogrpc.NekoServiceClient
	chatStream nekogrpc.NekoService_ChatClient
}

func main() {
	gui := &grpcGUI{}
	gui.app = fyneapp.NewWithID("neko_grpc_gui")
	gui.window = gui.app.NewWindow("neko_grpc GUI")
	gui.window.Resize(fyne.NewSize(1240, 820))
	gui.window.SetContent(gui.build())
	gui.window.ShowAndRun()
}

func (g *grpcGUI) build() fyne.CanvasObject {
	g.addrEntry = widget.NewEntry()
	g.addrEntry.SetText("127.0.0.1:17501")
	g.tokenEntry = widget.NewPasswordEntry()
	g.tokenEntry.SetText("dev-token")
	g.statusLabel = widget.NewLabel("Disconnected")

	g.keyEntry = widget.NewEntry()
	g.keyEntry.SetText("profile:1")
	g.valueEntry = widget.NewEntry()
	g.valueEntry.SetText(`{"name":"alice","mmr":1200}`)
	g.watchKeyEntry = widget.NewEntry()
	g.watchKeyEntry.SetText("profile:1")
	g.kvOutput = widget.NewMultiLineEntry()
	g.kvOutput.Disable()
	g.kvOutput.Wrapping = fyne.TextWrapWord
	g.kvOutput.SetMinRowsVisible(14)

	g.roomEntry = widget.NewEntry()
	g.roomEntry.SetText("lobby")
	g.userEntry = widget.NewEntry()
	g.userEntry.SetText("alice")
	g.messageEntry = widget.NewEntry()
	g.chatOutput = widget.NewMultiLineEntry()
	g.chatOutput.Disable()
	g.chatOutput.Wrapping = fyne.TextWrapWord
	g.chatOutput.SetMinRowsVisible(14)

	connectBtn := widget.NewButton("Connect", func() { go g.connect() })
	disconnectBtn := widget.NewButton("Disconnect", func() { g.disconnect() })

	putBtn := widget.NewButton("Put", func() { go g.put() })
	getBtn := widget.NewButton("Get", func() { go g.get() })
	watchBtn := widget.NewButton("Watch", func() { go g.watch() })

	chatConnectBtn := widget.NewButton("Open Chat", func() { go g.openChat() })
	chatSendBtn := widget.NewButton("Send", func() { go g.sendChat() })

	top := container.NewVBox(
		widget.NewLabelWithStyle("neko_grpc GUI", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabel("Server"),
		g.addrEntry,
		widget.NewLabel("Bearer token"),
		g.tokenEntry,
		container.NewGridWithColumns(2, connectBtn, disconnectBtn),
		g.statusLabel,
	)

	kvTab := container.NewBorder(
		container.NewVBox(
			widget.NewLabel("Key"),
			g.keyEntry,
			widget.NewLabel("JSON value"),
			g.valueEntry,
			container.NewGridWithColumns(3, putBtn, getBtn, watchBtn),
			widget.NewLabel("Watch key"),
			g.watchKeyEntry,
		),
		nil, nil, nil, g.kvOutput,
	)

	chatTab := container.NewBorder(
		container.NewVBox(
			widget.NewLabel("Room"),
			g.roomEntry,
			widget.NewLabel("User"),
			g.userEntry,
			container.NewGridWithColumns(2, chatConnectBtn, chatSendBtn),
			widget.NewLabel("Message"),
			g.messageEntry,
		),
		nil, nil, nil, g.chatOutput,
	)

	tabs := container.NewAppTabs(
		container.NewTabItem("KV", kvTab),
		container.NewTabItem("Chat", chatTab),
	)

	return container.NewBorder(top, nil, nil, nil, tabs)
}

func (g *grpcGUI) connect() {
	g.setStatus("Connecting...")
	conn, err := grpc.NewClient(strings.TrimSpace(g.addrEntry.Text),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
			d := net.Dialer{Timeout: 3 * time.Second}
			return d.DialContext(ctx, "tcp", addr)
		}),
	)
	if err != nil {
		g.showError(err)
		g.setStatus("Connect failed")
		return
	}
	if g.conn != nil {
		_ = g.conn.Close()
	}
	g.conn = conn
	g.client = nekogrpc.NewClient(conn)
	g.setStatus("Connected")
}

func (g *grpcGUI) disconnect() {
	if g.conn != nil {
		_ = g.conn.Close()
	}
	g.conn = nil
	g.client = nil
	g.chatStream = nil
	g.setStatus("Disconnected")
}

func (g *grpcGUI) authContext() (context.Context, context.CancelFunc, error) {
	if g.client == nil {
		return nil, nil, fmt.Errorf("connect first")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	token := strings.TrimSpace(g.tokenEntry.Text)
	if token != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
	}
	return ctx, cancel, nil
}

func (g *grpcGUI) put() {
	ctx, cancel, err := g.authContext()
	if err != nil {
		g.showError(err)
		return
	}
	defer cancel()

	value := map[string]any{}
	raw := strings.TrimSpace(g.valueEntry.Text)
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &value); err != nil {
			g.showError(fmt.Errorf("value must be valid JSON object: %w", err))
			return
		}
	}
	msg, err := structpb.NewStruct(map[string]any{
		"key":   strings.TrimSpace(g.keyEntry.Text),
		"value": value,
	})
	if err != nil {
		g.showError(err)
		return
	}
	resp, err := g.client.Put(ctx, msg)
	if err != nil {
		g.showError(err)
		return
	}
	g.setKVOutput(resp)
}

func (g *grpcGUI) get() {
	ctx, cancel, err := g.authContext()
	if err != nil {
		g.showError(err)
		return
	}
	defer cancel()
	resp, err := g.client.Get(ctx, nekogrpc.MustStruct(map[string]any{"key": strings.TrimSpace(g.keyEntry.Text)}))
	if err != nil {
		g.showError(err)
		return
	}
	g.setKVOutput(resp)
}

func (g *grpcGUI) watch() {
	ctx, cancel, err := g.authContext()
	if err != nil {
		g.showError(err)
		return
	}
	stream, err := g.client.Watch(ctx, nekogrpc.MustStruct(map[string]any{"key": strings.TrimSpace(g.watchKeyEntry.Text)}))
	if err != nil {
		cancel()
		g.showError(err)
		return
	}
	g.setStatus("Watching " + strings.TrimSpace(g.watchKeyEntry.Text))
	go func() {
		defer cancel()
		for {
			msg, err := stream.Recv()
			if err != nil {
				fyne.Do(func() {
					g.appendKVLog("watch ended: " + err.Error())
				})
				return
			}
			fyne.Do(func() {
				g.appendKVLog(prettyStruct(msg))
			})
		}
	}()
}

func (g *grpcGUI) openChat() {
	ctx, cancel, err := g.authContext()
	if err != nil {
		g.showError(err)
		return
	}
	stream, err := g.client.Chat(ctx)
	if err != nil {
		cancel()
		g.showError(err)
		return
	}
	g.chatStream = stream
	g.setStatus("Chat opened")
	go func() {
		defer cancel()
		for {
			msg, err := stream.Recv()
			if err != nil {
				fyne.Do(func() { g.appendChat("chat ended: " + err.Error()) })
				return
			}
			fyne.Do(func() { g.appendChat(prettyStruct(msg)) })
		}
	}()
}

func (g *grpcGUI) sendChat() {
	if g.chatStream == nil {
		g.showError(fmt.Errorf("open chat first"))
		return
	}
	msg := nekogrpc.MustStruct(map[string]any{
		"room":    strings.TrimSpace(g.roomEntry.Text),
		"user":    strings.TrimSpace(g.userEntry.Text),
		"message": strings.TrimSpace(g.messageEntry.Text),
	})
	if err := g.chatStream.Send(msg); err != nil {
		g.showError(err)
		return
	}
	fyne.Do(func() { g.messageEntry.SetText("") })
}

func (g *grpcGUI) setStatus(text string) {
	fyne.Do(func() { g.statusLabel.SetText(text) })
}

func (g *grpcGUI) setKVOutput(msg *structpb.Struct) {
	fyne.Do(func() {
		g.kvOutput.SetText(prettyStruct(msg))
	})
}

func (g *grpcGUI) appendKVLog(line string) {
	text := g.kvOutput.Text
	if text != "" {
		text += "\n\n"
	}
	g.kvOutput.SetText(text + line)
}

func (g *grpcGUI) appendChat(line string) {
	text := g.chatOutput.Text
	if text != "" {
		text += "\n"
	}
	g.chatOutput.SetText(text + line)
}

func (g *grpcGUI) showError(err error) {
	fyne.Do(func() { dialog.ShowError(err, g.window) })
}

func prettyStruct(msg *structpb.Struct) string {
	if msg == nil {
		return "{}"
	}
	data, err := json.MarshalIndent(msg.AsMap(), "", "  ")
	if err != nil {
		return err.Error()
	}
	return string(data)
}
