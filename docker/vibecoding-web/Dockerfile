# VibeCoding Web Interface Dockerfile
# Внешний контейнер для веб-интерфейса с MCP коммуникацией

FROM node:18-alpine

WORKDIR /app

# Устанавливаем зависимости для веб-интерфейса
RUN npm install -g express cors ws

# Копируем веб-интерфейс
COPY web/ ./web/
COPY package.json ./
COPY server.js ./

# Устанавливаем зависимости
RUN npm install

# Открываем порт для веб-интерфейса
EXPOSE 3000

# Запускаем веб-сервер с MCP коммуникацией
CMD ["node", "server.js"]