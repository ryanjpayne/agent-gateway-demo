FROM python:3.13-slim

WORKDIR /app

# Install dependencies
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Copy all files from the current directory
COPY . .

# Expose port (Cloud Run defaults to 8080)
EXPOSE 8080

# Set standard environment variables
ENV PORT=8080
ENV HOST=0.0.0.0

# Run the SSE server
CMD ["python", "server.py"]
