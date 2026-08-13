# Generates small sine-tone "audiobooks" for pipeline testing (Windows dev).
# Usage: .\testdata\make-fixtures.ps1 [-InputDir devdata\input]
param([string]$InputDir = "devdata\input")

$ErrorActionPreference = 'Stop'

function New-Tone {
    param([string]$Path, [int]$Freq, [int]$Seconds, [string]$CodecArgs)
    $dir = Split-Path $Path -Parent
    if (-not (Test-Path $dir)) { New-Item -ItemType Directory -Force $dir | Out-Null }
    $args = @('-hide_banner', '-loglevel', 'error', '-y',
              '-f', 'lavfi', '-i', "sine=frequency=${Freq}:duration=${Seconds}") +
            ($CodecArgs -split ' ') + @($Path)
    & ffmpeg @args
    if ($LASTEXITCODE -ne 0) { throw "ffmpeg failed for $Path" }
}

# 1) Multi-mp3 book (transcode path, chapter-per-file fallback)
1..3 | ForEach-Object {
    New-Tone -Path "$InputDir\Real Sine Book\0$_ - Movement $_.mp3" -Freq (330 + 110 * $_) -Seconds 20 -CodecArgs '-c:a libmp3lame -b:a 64k -ar 44100'
}

# 2) Multi-disc m4a book (stream-copy concat path)
1..2 | ForEach-Object { $disc = $_
    1..2 | ForEach-Object {
        New-Tone -Path "$InputDir\Sine Disc Saga\CD$disc\track_$_.m4a" -Freq (200 + 100 * $disc + 50 * $_) -Seconds 15 -CodecArgs '-c:a aac -b:a 96k -ar 44100'
    }
}

# 3) Single m4b (copy + retag path)
New-Tone -Path "$InputDir\Solo Sine.m4b" -Freq 550 -Seconds 30 -CodecArgs '-c:a aac -b:a 96k -ar 44100'

Write-Host "fixtures written to $InputDir"
