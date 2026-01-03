FROM node:22-slim

# Create workspace directory
WORKDIR /workspace

# Copy workspace package files (for workspaces setup)
COPY package*.json ./
COPY frontend/ ./frontend/

# Install dependencies at workspace root (will link all workspaces)
RUN npm install

# Set working directory to frontend
WORKDIR /workspace/frontend

# Create data directory for persistence
RUN mkdir -p /data

# Expose port
EXPOSE 5173

# Start the service
CMD ["yarn", "dev"]
