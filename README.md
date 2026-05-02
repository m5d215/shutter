# shutter

A small HTTP daemon that returns a JPEG frame from an avfoundation camera via `ffmpeg`. By default it binds to `127.0.0.1:9998` only — to reach it from another host, front it with a reverse proxy or VPN (Tailscale Serve, ngrok, an ssh tunnel, etc.).

## Requirements

- macOS (verified on Apple Silicon)
- `ffmpeg` installed (Homebrew puts it at `/opt/homebrew/bin/ffmpeg`)
- A camera connected and visible in `ffmpeg -f avfoundation -list_devices true -i ""`
- **Launch from a Terminal app**. macOS TCC (Privacy → Camera) silently denies access when launched over ssh
- Allow the camera permission prompt on first launch

## Install

### Homebrew (recommended)

```
brew tap m5d215/tap
brew install shutter
```

A config file is created at `$(brew --prefix)/etc/shutter.conf`. At a minimum, set `SHUTTER_DEVICE`:

```
vim "$(brew --prefix)/etc/shutter.conf"
```

```
SHUTTER_DEVICE=MX Brio
```

To grant camera permission, run `shutter` once from a Terminal:

```
shutter
```

Approve the dialog, stop it with `Ctrl-C`, then run it under launchd:

```
brew services start shutter
```

After `brew upgrade`, the binary signature changes and TCC will revoke access. You'll need to launch `shutter` from a Terminal again to re-grant permission. This is a macOS limitation.

### From source

```
go build -o shutter
SHUTTER_DEVICE="MX Brio" ./shutter
```

## Configuration

Settings are resolved in this order (highest priority first):

1. Process environment variables
2. `KEY=VALUE` file pointed to by `SHUTTER_CONFIG`
3. Built-in defaults

| Variable | Default | Description |
|---|---|---|
| `SHUTTER_DEVICE` | (required) | avfoundation device name |
| `SHUTTER_LISTEN_ADDR` | `127.0.0.1:9998` | Listen address |
| `SHUTTER_FFMPEG` | `/opt/homebrew/bin/ffmpeg` | Path to the `ffmpeg` binary |
| `SHUTTER_VIDEO_SIZE` | `3840x2160` | Capture resolution |
| `SHUTTER_FRAMERATE` | `30` | Capture framerate |
| `SHUTTER_CAPTURE_TIMEOUT` | `10s` | Abort `ffmpeg` if it doesn't respond within this duration |
| `SHUTTER_CONFIG` | (unset) | Path to the config file. Set automatically to `etc/shutter.conf` when installed via Homebrew |

Example config file:

```
# /opt/homebrew/etc/shutter.conf
SHUTTER_DEVICE=MX Brio
# SHUTTER_VIDEO_SIZE=1920x1080
# SHUTTER_FRAMERATE=30
```

Lines starting with `#` and blank lines are ignored.

## Usage

```
curl -s http://127.0.0.1:9998/capture -o /tmp/test.jpg
file /tmp/test.jpg      # => JPEG image data, ...
open /tmp/test.jpg
```

## Endpoints

- `GET /capture`
  - Success: `200`, `Content-Type: image/jpeg`, body = one JPEG frame
  - Failure: `500` (or `504` if `ffmpeg` hangs), `Content-Type: text/plain; charset=utf-8`, body = error message

## Limitations

- The camera is exclusive per device. Concurrent requests are serialized via an in-process mutex
- No application-level auth. Loopback bind is the default; if you expose it externally, terminate auth at the fronting layer
- No streaming (one request = one frame)

## Logs

stdout/stderr, one line per event:

- `req GET /capture from 127.0.0.1:xxxx`
- `capture ok bytes=... dur=...`
- `capture failed status=... dur=... err=...`

## License

MIT — see `LICENSE`.
