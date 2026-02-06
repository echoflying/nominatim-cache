# 基础镜像
FROM node:20-alpine

# 工作目录
WORKDIR /app

# 安装依赖并构建
COPY package*.json ./
RUN npm ci --only=production

# 复制源代码并构建
COPY . .
RUN npm run build

# 安装 curl 用于健康检查
RUN apk add --no-cache curl

# 暴露端口
EXPOSE 3000

# 启动命令
CMD ["node", "dist/index.js"]
