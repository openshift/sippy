# Google Gemini Integration Setup

This document explains how to configure the Sippy Agent to use Google Gemini models with either API keys or service account credentials.

## Authentication Methods

You can authenticate with Google Gemini using either:
1. **API Key** (simpler, for development)
2. **Service Account Credentials** (recommended for production)

## Method 1: API Key Authentication

### 1. Get a Google API Key

1. Go to the [Google AI Studio](https://aistudio.google.com/)
2. Click "Get API key"
3. Create a new API key or use an existing one
4. Copy the API key

### 2. Configure Environment

Add to your `.env` file:
```bash
MODEL_NAME=gemini-1.5-pro
GOOGLE_API_KEY=your_api_key_here
```

### 3. Run the Agent

```bash
# CLI
python main.py --model gemini-1.5-pro

# Web Server
python web_main.py --model gemini-1.5-pro
```

## Method 2: Service Account Authentication

### 1. Create a Service Account

1. Go to the [Google Cloud Console](https://console.cloud.google.com/)
2. Select or create a project
3. Go to "IAM & Admin" > "Service Accounts"
4. Click "Create Service Account"
5. Fill in the details and click "Create"
6. Grant the service account the "Generative AI User" role
7. Click "Done"

### 2. Create and Download Credentials

1. Click on the created service account
2. Go to the "Keys" tab
3. Click "Add Key" > "Create new key"
4. Choose "JSON" format
5. Download the JSON file
6. Store it securely (e.g., `~/.config/gcloud/sippy-agent-credentials.json`)

### 3. Configure Environment

Add to your `.env` file:
```bash
MODEL_NAME=gemini-1.5-pro
GOOGLE_APPLICATION_CREDENTIALS=/path/to/your/service-account-key.json
```

### 4. Run the Agent

```bash
# CLI
python main.py --model gemini-1.5-pro --google-credentials /path/to/credentials.json

# Web Server
python web_main.py --model gemini-1.5-pro --google-credentials /path/to/credentials.json
```

## Available Gemini Models

- `gemini-1.5-pro` - Most capable model
- `gemini-1.5-flash` - Faster, optimized for speed
- `gemini-1.0-pro` - Previous generation

## Configuration Options

### Environment Variables

```bash
# Required
MODEL_NAME=gemini-1.5-pro

# Authentication (choose one)
GOOGLE_API_KEY=your_api_key_here
# OR
GOOGLE_APPLICATION_CREDENTIALS=/path/to/service-account-key.json

# Optional
TEMPERATURE=0.1
MAX_ITERATIONS=25
MAX_EXECUTION_TIME=1800
```

### Command Line Options

```bash
# Using API key
python main.py \
  --model gemini-1.5-pro \
  --temperature 0.1

# Using service account
python main.py \
  --model gemini-1.5-pro \
  --google-credentials /path/to/credentials.json \
  --temperature 0.1
```

## Security Best Practices

### For API Keys:
- Never commit API keys to version control
- Use environment variables or `.env` files
- Rotate keys regularly
- Restrict API key usage to specific IPs if possible

### For Service Accounts:
- Store credential files outside the project directory
- Set appropriate file permissions (600)
- Use IAM roles with minimal required permissions
- Rotate service account keys regularly

```bash
# Set secure permissions
chmod 600 /path/to/service-account-key.json

# Store in a secure location
mkdir -p ~/.config/gcloud
mv service-account-key.json ~/.config/gcloud/sippy-agent-credentials.json
```

## Troubleshooting

### Common Issues:

1. **"Google API key is required"**
   - Ensure `GOOGLE_API_KEY` or `GOOGLE_APPLICATION_CREDENTIALS` is set
   - Check that the model name starts with "gemini"

2. **"Permission denied"**
   - Verify the API key has the correct permissions
   - For service accounts, ensure the "Generative AI User" role is assigned

3. **"Model not found"**
   - Check that the model name is correct (e.g., `gemini-1.5-pro`)
   - Ensure the model is available in your region

4. **"Quota exceeded"**
   - Check your Google Cloud billing and quotas
   - Consider using `gemini-1.5-flash` for higher throughput

### Debug Mode:

Run with verbose logging to see detailed information:

```bash
python main.py --model gemini-1.5-pro --verbose
```

## Example Usage

```bash
# Start CLI with Gemini
python main.py --model gemini-1.5-pro --thinking

# Start web server with Gemini
python web_main.py --model gemini-1.5-pro --port 8000

# Example queries
"Analyze job 1934795512955801600"
"What's the status of payload 4.17.0-0.nightly-2024-01-15-123456?"
"Check for known incidents related to registry failures"
```

## Cost Considerations

- Gemini models have different pricing tiers
- `gemini-1.5-flash` is more cost-effective for high-volume usage
- Monitor usage through Google Cloud Console
- Consider setting up billing alerts

For the latest pricing information, visit the [Google AI Pricing page](https://ai.google.dev/pricing).
