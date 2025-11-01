# bc-vod-urls

A command-line utility to generate VOD (Video on Demand) URLs from Brightcove live playback URLs.

## Description

This tool retrieves all session information from a Brightcove live stream resource and generates corresponding VOD playback URLs. It handles authentication, session retrieval, playback token generation, and URL construction automatically.

## Prerequisites

- Go 1.24.1 or later
- Brightcove API credentials (Client ID and Client Secret)

## Installation

```bash
go build -o vodurls main.go
```

## Configuration

Create a `.env` file in the project root with your Brightcove API credentials:

```
CLIENT_ID=your_client_id_here
CLIENT_SECRET=your_client_secret_here
```

## Usage

```bash
./vodurls <PLAYBACK_URL>
```

**Example:**

```bash
./vodurls https://fastly.live.brightcove.com/6384185469112/ap-south-1/6415518627001/eyJhbGciOiJIUzI1NiIsInR5cCI6I...
```

The tool will output VOD URLs for each session found:

```
VOD URL[0]: https://...
VOD URL[1]: https://...
```

## How It Works

1. Authenticates with Brightcove OAuth API using client credentials
2. Extracts resource and account IDs from the playback URL
3. Retrieves all sessions associated with the resource
4. Generates playback tokens for each session (HLS format)
5. Constructs and returns VOD playback URLs

## Dependencies

- [godotenv](https://github.com/joho/godotenv) - Environment variable management

