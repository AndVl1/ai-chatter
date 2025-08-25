package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// GitHubReleaseParams параметры для получения релизов GitHub
type GitHubReleaseParams struct {
	Owner          string `json:"owner" mcp:"GitHub repository owner (e.g., 'AndVl1')"`
	Repo           string `json:"repo" mcp:"GitHub repository name (e.g., 'SnakeGame')"`
	MaxReleases    int    `json:"max_releases,omitempty" mcp:"maximum number of releases to return (default: 10, max: 50)"`
	IncludeDrafts  bool   `json:"include_drafts,omitempty" mcp:"include draft releases (default: false)"`
	PreReleaseOnly bool   `json:"prerelease_only,omitempty" mcp:"only pre-releases (default: false)"`
}

// GitHubDownloadAssetParams параметры для скачивания ассета
type GitHubDownloadAssetParams struct {
	Owner      string `json:"owner" mcp:"GitHub repository owner"`
	Repo       string `json:"repo" mcp:"GitHub repository name"`
	ReleaseID  int64  `json:"release_id" mcp:"GitHub release ID"`
	AssetName  string `json:"asset_name" mcp:"name of the asset to download (e.g., 'app-release.aab')"`
	TargetPath string `json:"target_path,omitempty" mcp:"local path to save the file (optional)"`
}

// GitHubRelease информация о релизе GitHub
type GitHubRelease struct {
	ID           int64                `json:"id"`
	TagName      string               `json:"tag_name"`
	Name         string               `json:"name"`
	Body         string               `json:"body"`
	IsDraft      bool                 `json:"draft"`
	IsPrerelease bool                 `json:"prerelease"`
	CreatedAt    time.Time            `json:"created_at"`
	PublishedAt  time.Time            `json:"published_at"`
	Assets       []GitHubReleaseAsset `json:"assets"`
	Author       GitHubUser           `json:"author"`
	HTMLURL      string               `json:"html_url"`
}

// GitHubReleaseAsset информация об ассете релиза
type GitHubReleaseAsset struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Label       string    `json:"label"`
	Size        int64     `json:"size"`
	DownloadURL string    `json:"browser_download_url"`
	ContentType string    `json:"content_type"`
	State       string    `json:"state"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// GitHubUser информация о пользователе GitHub
type GitHubUser struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	AvatarURL string `json:"avatar_url"`
	HTMLURL   string `json:"html_url"`
}

// GitHubMCPServer кастомный MCP сервер для GitHub
type GitHubMCPServer struct {
	client *http.Client
	token  string
}

// NewGitHubMCPServer создает новый MCP сервер для GitHub
func NewGitHubMCPServer(token string) (*GitHubMCPServer, error) {
	log.Printf("🔑 Initializing GitHub MCP Server")

	if token == "" {
		log.Printf("⚠️ Warning: No GitHub token provided, using public API (rate limited)")
	}

	return &GitHubMCPServer{
		client: &http.Client{Timeout: 30 * time.Second},
		token:  token,
	}, nil
}

// makeGitHubRequest выполняет HTTP запрос к GitHub API
func (g *GitHubMCPServer) makeGitHubRequest(ctx context.Context, url string) (*http.Response, error) {
	log.Printf("🔗 GitHub API: Making request to %s", url)
	log.Printf("🔑 GitHub API: Using authentication: %v", g.token != "")

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		log.Printf("❌ GitHub API: Failed to create request: %v", err)
		return nil, err
	}

	// Добавляем заголовки
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "ai-chatter-github-mcp/1.0.0")

	if g.token != "" {
		req.Header.Set("Authorization", "token "+g.token)
		log.Printf("🔑 GitHub API: Authorization header set with token")
		// Показываем маскированный токен для отладки
		if len(g.token) > 8 {
			maskedToken := g.token[:4] + "..." + g.token[len(g.token)-4:]
			log.Printf("🔑 GitHub API: Token: %s", maskedToken)
		}
	} else {
		log.Printf("⚠️ GitHub API: No token provided, using public API (rate limited)")
	}

	resp, err := g.client.Do(req)
	if err != nil {
		log.Printf("❌ GitHub API: Request failed: %v", err)
		return nil, err
	}

	log.Printf("📊 GitHub API: Response status: %d", resp.StatusCode)
	return resp, nil
}

// GetReleases получает список релизов репозитория
func (g *GitHubMCPServer) GetReleases(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[GitHubReleaseParams]) (*mcp.CallToolResultFor[any], error) {
	args := params.Arguments

	log.Printf("📦 MCP Server: Getting GitHub releases for %s/%s", args.Owner, args.Repo)

	// Устанавливаем лимит по умолчанию
	maxResults := args.MaxReleases
	if maxResults <= 0 {
		maxResults = 10
	}
	if maxResults > 50 {
		maxResults = 50
	}

	// Формируем URL
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases?per_page=%d", args.Owner, args.Repo, maxResults)

	resp, err := g.makeGitHubRequest(ctx, url)
	if err != nil {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("❌ GitHub API request failed: %v", err)},
			},
		}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("❌ GitHub API error %d: %s", resp.StatusCode, string(body))},
			},
		}, nil
	}

	// Парсим ответ
	var releases []GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("❌ Failed to parse GitHub response: %v", err)},
			},
		}, nil
	}

	// Фильтруем релизы
	var filteredReleases []GitHubRelease
	for _, release := range releases {
		// Пропускаем драфты если не нужны
		if release.IsDraft && !args.IncludeDrafts {
			continue
		}

		// Если нужны только пре-релизы
		if args.PreReleaseOnly && !release.IsPrerelease {
			continue
		}

		filteredReleases = append(filteredReleases, release)
	}

	// Формируем ответ
	var resultMessage string
	if len(filteredReleases) == 0 {
		resultMessage = fmt.Sprintf("📦 No releases found for %s/%s with specified filters", args.Owner, args.Repo)
	} else {
		resultMessage = fmt.Sprintf("📦 Found %d releases for %s/%s:\n\n", len(filteredReleases), args.Owner, args.Repo)
		for i, release := range filteredReleases {
			releaseType := ""
			if release.IsDraft {
				releaseType = " 📝 (Draft)"
			} else if release.IsPrerelease {
				releaseType = " 🧪 (Pre-release)"
			}

			resultMessage += fmt.Sprintf("%d. **%s** (%s)%s\n", i+1, release.Name, release.TagName, releaseType)
			resultMessage += fmt.Sprintf("   **ID:** %d\n", release.ID)
			resultMessage += fmt.Sprintf("   **Published:** %s\n", release.PublishedAt.Format("2006-01-02 15:04"))
			resultMessage += fmt.Sprintf("   **Author:** @%s\n", release.Author.Login)

			if len(release.Assets) > 0 {
				resultMessage += fmt.Sprintf("   **Assets (%d):**\n", len(release.Assets))
				for _, asset := range release.Assets {
					sizeKB := asset.Size / 1024
					resultMessage += fmt.Sprintf("     - %s (%d KB)\n", asset.Name, sizeKB)
				}
			}

			if release.Body != "" {
				// Обрезаем описание если слишком длинное
				body := release.Body
				if len(body) > 200 {
					body = body[:200] + "..."
				}
				resultMessage += fmt.Sprintf("   **Description:** %s\n", body)
			}

			resultMessage += fmt.Sprintf("   **URL:** %s\n\n", release.HTMLURL)
		}
	}

	return &mcp.CallToolResultFor[any]{
		Content: []mcp.Content{
			&mcp.TextContent{Text: resultMessage},
		},
		Meta: map[string]interface{}{
			"owner":       args.Owner,
			"repo":        args.Repo,
			"releases":    filteredReleases,
			"total_found": len(filteredReleases),
			"success":     true,
		},
	}, nil
}

// DownloadAsset скачивает ассет релиза
func (g *GitHubMCPServer) DownloadAsset(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[GitHubDownloadAssetParams]) (*mcp.CallToolResultFor[any], error) {
	args := params.Arguments

	log.Printf("⬇️ MCP Server: Downloading asset %s for release %d from %s/%s", args.AssetName, args.ReleaseID, args.Owner, args.Repo)

	// Получаем информацию о релизе
	releaseURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/%d", args.Owner, args.Repo, args.ReleaseID)

	resp, err := g.makeGitHubRequest(ctx, releaseURL)
	if err != nil {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("❌ Failed to get release info: %v", err)},
			},
		}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("❌ GitHub API error %d: %s", resp.StatusCode, string(body))},
			},
		}, nil
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("❌ Failed to parse release info: %v", err)},
			},
		}, nil
	}

	// Ищем нужный ассет
	var targetAsset *GitHubReleaseAsset
	for _, asset := range release.Assets {
		if asset.Name == args.AssetName || strings.Contains(asset.Name, args.AssetName) {
			targetAsset = &asset
			break
		}
	}

	if targetAsset == nil {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("❌ Asset '%s' not found in release %s", args.AssetName, release.TagName)},
			},
		}, nil
	}

	// Скачиваем ассет
	downloadResp, err := g.makeGitHubRequest(ctx, targetAsset.DownloadURL)
	if err != nil {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("❌ Failed to download asset: %v", err)},
			},
		}, nil
	}
	defer downloadResp.Body.Close()

	if downloadResp.StatusCode != http.StatusOK {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("❌ Download failed with status %d", downloadResp.StatusCode)},
			},
		}, nil
	}

	// Читаем содержимое файла
	fileData, err := io.ReadAll(downloadResp.Body)
	if err != nil {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("❌ Failed to read downloaded file: %v", err)},
			},
		}, nil
	}

	// Определяем путь для сохранения
	targetPath := args.TargetPath
	if targetPath == "" {
		targetPath = fmt.Sprintf("./downloads/%s", targetAsset.Name)
	}

	// Создаем директорию если не существует
	if dir := strings.TrimSuffix(targetPath, targetAsset.Name); dir != "" && dir != "./" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			log.Printf("⚠️ Warning: failed to create directory %s: %v", dir, err)
		}
	}

	// Сохраняем файл
	if err := os.WriteFile(targetPath, fileData, 0644); err != nil {
		return &mcp.CallToolResultFor[any]{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("❌ Failed to save file: %v", err)},
			},
		}, nil
	}

	// Кодируем содержимое в base64 для передачи
	base64Content := base64.StdEncoding.EncodeToString(fileData)

	resultMessage := fmt.Sprintf("✅ Successfully downloaded asset '%s' from release %s\n", targetAsset.Name, release.TagName)
	resultMessage += fmt.Sprintf("**File size:** %d bytes (%.2f KB)\n", len(fileData), float64(len(fileData))/1024)
	resultMessage += fmt.Sprintf("**Saved to:** %s\n", targetPath)
	resultMessage += fmt.Sprintf("**Content type:** %s\n", targetAsset.ContentType)

	return &mcp.CallToolResultFor[any]{
		Content: []mcp.Content{
			&mcp.TextContent{Text: resultMessage},
		},
		Meta: map[string]interface{}{
			"success":        true,
			"asset_name":     targetAsset.Name,
			"asset_size":     len(fileData),
			"target_path":    targetPath,
			"content_type":   targetAsset.ContentType,
			"base64_content": base64Content,
			"release":        release,
		},
	}, nil
}

func main() {
	if err := godotenv.Load(".env"); err != nil {
		log.Printf("Warning: .env file not found: %v", err)
	}

	// Получаем GitHub токен из переменной окружения
	githubToken := os.Getenv("GITHUB_TOKEN")

	log.Printf("🚀 Starting GitHub MCP Server")
	log.Printf("📦 GitHub token available: %v", githubToken != "")

	// Добавляем диагностику токена
	if githubToken == "" {
		log.Printf("⚠️ GITHUB_TOKEN environment variable is empty!")
		log.Printf("💡 Please set GITHUB_TOKEN before starting the server")
		log.Printf("🔗 GitHub will use public API (highly rate limited)")
	} else {
		// Показываем только первые и последние 4 символа токена для безопасности
		tokenLen := len(githubToken)
		if tokenLen > 8 {
			maskedToken := githubToken[:4] + "..." + githubToken[tokenLen-4:]
			log.Printf("🔑 GitHub token: %s (length: %d)", maskedToken, tokenLen)
		} else {
			log.Printf("🔑 GitHub token length: %d", tokenLen)
		}

		// Проверяем формат токена
		if strings.HasPrefix(githubToken, "ghp_") {
			log.Printf("✅ Personal Access Token format detected")
		} else if strings.HasPrefix(githubToken, "github_pat_") {
			log.Printf("✅ Fine-grained Personal Access Token format detected")
		} else if strings.HasPrefix(githubToken, "gho_") {
			log.Printf("✅ OAuth token format detected")
		} else {
			log.Printf("⚠️ Unexpected token format - should start with ghp_, github_pat_, or gho_")
		}
	}

	// Создаем GitHub сервер
	githubServer, err := NewGitHubMCPServer(githubToken)
	if err != nil {
		log.Fatalf("❌ Failed to create GitHub server: %v", err)
	}

	// Создаем MCP сервер
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "ai-chatter-github-mcp",
		Version: "1.0.0",
	}, nil)

	// Регистрируем инструменты
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_github_releases",
		Description: "Gets list of releases from a GitHub repository with filtering options",
	}, githubServer.GetReleases)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "download_github_asset",
		Description: "Downloads an asset (file) from a GitHub release",
	}, githubServer.DownloadAsset)

	log.Printf("📋 Registered GitHub MCP tools: get_github_releases, download_github_asset")
	log.Printf("🔗 Starting GitHub MCP server on stdin/stdout...")

	// Запускаем сервер через stdin/stdout
	transport := mcp.NewStdioTransport()
	if err := server.Run(context.Background(), transport); err != nil {
		log.Fatalf("❌ GitHub MCP Server failed: %v", err)
	}
}
