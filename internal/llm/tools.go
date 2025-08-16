package llm

// GetNotionTools возвращает список инструментов Notion для LLM
func GetNotionTools() []Tool {
	return []Tool{
		{
			Type: "function",
			Function: Function{
				Name:        "save_dialog_to_notion",
				Description: "Сохраняет текущий диалог в Notion. Используется когда пользователь просит сохранить беседу, запомнить информацию или создать заметку.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"title": map[string]interface{}{
							"type":        "string",
							"description": "Название страницы в Notion (краткое и понятное)",
						},
					},
					"required": []string{"title"},
				},
			},
		},
		{
			Type: "function",
			Function: Function{
				Name:        "search_notion",
				Description: "Ищет информацию в ранее сохранённых диалогах в Notion. Используется когда пользователь спрашивает про прошлые беседы или нужно найти ранее обсуждённые темы.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"query": map[string]interface{}{
							"type":        "string",
							"description": "Поисковый запрос для поиска в Notion",
						},
					},
					"required": []string{"query"},
				},
			},
		},
		{
			Type: "function",
			Function: Function{
				Name:        "create_notion_page",
				Description: "Создаёт новую страницу в Notion с произвольным содержимым. Используется для создания заметок, планов, или структурированной информации. Можно указать parent_page_id для создания подстраницы. Если не знаешь доступные страницы, используй list_available_pages сначала.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"title": map[string]interface{}{
							"type":        "string",
							"description": "Название страницы",
						},
						"content": map[string]interface{}{
							"type":        "string",
							"description": "Содержимое страницы в формате Markdown",
						},
						"parent_page_id": map[string]interface{}{
							"type":        "string",
							"description": "ID родительской страницы для создания подстраницы (опционально). Если не указано, используется default parent page.",
						},
					},
					"required": []string{"title", "content"},
				},
			},
		},
		{
			Type: "function",
			Function: Function{
				Name:        "search_pages_with_id",
				Description: "Ищет страницы в Notion по названию и возвращает их ID, заголовок и URL. Используется когда нужно найти страницу по названию для получения её ID или когда пользователь ищет конкретную страницу.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"query": map[string]interface{}{
							"type":        "string",
							"description": "Поисковый запрос - название страницы или ключевые слова",
						},
						"exact_match": map[string]interface{}{
							"type":        "boolean",
							"description": "Если true, искать только точные совпадения названия (по умолчанию false)",
						},
						"limit": map[string]interface{}{
							"type":        "integer",
							"description": "Максимальное количество результатов (по умолчанию 5, максимум 20)",
						},
					},
					"required": []string{"query"},
				},
			},
		},
		{
			Type: "function",
			Function: Function{
				Name:        "list_available_pages",
				Description: "Получает список доступных страниц в Notion workspace, которые могут использоваться как родительские страницы. Используется для выбора подходящей родительской страницы при создании новых страниц.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"limit": map[string]interface{}{
							"type":        "integer",
							"description": "Максимальное количество страниц для возврата (по умолчанию 10, максимум 25)",
						},
						"page_type": map[string]interface{}{
							"type":        "string",
							"description": "Фильтр по типу страницы (опционально)",
						},
						"parent_only": map[string]interface{}{
							"type":        "boolean",
							"description": "Если true, возвращать только страницы которые могут быть родителями (по умолчанию false)",
						},
					},
					"required": []string{},
				},
			},
		},
	}
}
