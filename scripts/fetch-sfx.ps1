# fetch-sfx.ps1 - re-downloads the sound effects from Sprite Fusion's
# "Destroy Any Website" game (https://destroy.spritefusion.com/) and
# extracts the six DUKE cues from its chip.flac sample atlas.
#
# Requires: PowerShell + ffmpeg on PATH.
# Usage:    ./scripts/fetch-sfx.ps1
#
# The atlas + manifest land in sounds/_src/ (gitignored). To switch styles,
# change $baseUrl/$set (the site has other sets: see /assets/sfx/<set>.json)
# and/or remap the cues below.
$baseUrl = 'https://destroy.spritefusion.com/assets/sfx'
$set = 'chip'

New-Item -ItemType Directory -Force -Path ./sounds/_src | Out-Null

Invoke-WebRequest -Uri "$baseUrl/$set.json" -OutFile "./sounds/_src/$set.json" -UseBasicParsing
Invoke-WebRequest -Uri "$baseUrl/$set.flac" -OutFile "./sounds/_src/$set.flac" -UseBasicParsing
$json = Get-Content "./sounds/_src/$set.json" -Raw | ConvertFrom-Json
Write-Host "set: $($json.name) - $($json.blurb)"

# DUKE cue -> (atlas sound name, variant index)
$map = [ordered]@{
	'pistol.wav'  = @('pistol', 0)     # pistol shot
	'shotgun.wav' = @('shotgun', 0)    # shotgun blast
	'rocket.wav'  = @('rocket', 0)     # rocket launcher
	'slash.wav'   = @('starSweep', 0)  # katana whoosh
	'stamp.wav'   = @('hitPaper', 0)   # letter hits the paper buffer
	'save.wav'    = @('complete', 0)   # victory jingle
}

$src = "./sounds/_src/$set.flac"
foreach ($out in $map.Keys) {
	$snd = $map[$out][0]
	$v = $map[$out][1]
	$seg = $json.sounds.$snd
	if (-not $seg) { Write-Warning "missing sound: $snd"; continue }
	$start = $seg[$v][0]
	$len = $seg[$v][1]
	$ss = $start / $json.sampleRate
	$dur = $len / $json.sampleRate
	ffmpeg -hide_banner -loglevel error -y -i $src -ss $ss -t $dur `
		-acodec pcm_s16le -ar $json.sampleRate "./sounds/$out"
	if ($LASTEXITCODE -eq 0) {
		Write-Host "OK $out <- $snd[$v] ($([math]::Round($dur, 2))s)"
	} else {
		Write-Warning "ffmpeg failed for $out"
	}
}
