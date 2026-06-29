#requires -version 5.1
<#
.SYNOPSIS
  Bootstrap the OpenMeter/Konnect metering catalog for the clearinghouse collector.

.DESCRIPTION
  Idempotent: creates only what is missing. Meters are immutable in OpenMeter, so this
  script never updates or deletes them. Uses `kongctl api` against /v3/openmeter/*.

  Auth:     $env:KONGCTL_DEFAULT_KONNECT_PAT (preferred) or $env:OPENMETER_API_KEY (Konnect kpat_…).
  Endpoint: $env:OPENMETER_URL (default https://us.api.konghq.com/v3/openmeter).

.EXAMPLE
  .\bootstrap.ps1 catalog
.EXAMPLE
  .\bootstrap.ps1 customer demo-client demo-user "Demo User"
.EXAMPLE
  .\bootstrap.ps1 all demo-client demo-user "Demo User" -Subscribe
#>
[CmdletBinding()]
param(
  [ValidateSet('catalog', 'customer', 'all')]
  [string]$Command = 'catalog',
  [string]$ClientId,
  [string]$ExternalUserId,
  [string]$DisplayName,
  [switch]$Subscribe
)

$ErrorActionPreference = 'Stop'
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$Catalog = if ($env:CATALOG) { $env:CATALOG } else { Join-Path $ScriptDir 'catalog.json' }

function Die($m) { Write-Error $m; exit 1 }
function Info($m) { Write-Host $m }
function Warn($m) { Write-Warning $m }

if (-not (Get-Command kongctl -ErrorAction SilentlyContinue)) { Die 'kongctl not found (https://developer.konghq.com/kongctl/)' }
if (-not (Test-Path $Catalog)) { Die "catalog not found: $Catalog" }

# --- auth + endpoint -------------------------------------------------------
$pat = if ($env:KONGCTL_DEFAULT_KONNECT_PAT) { $env:KONGCTL_DEFAULT_KONNECT_PAT } else { $env:OPENMETER_API_KEY }
if (-not $pat) { Die 'set KONGCTL_DEFAULT_KONNECT_PAT or OPENMETER_API_KEY (a Konnect PAT)' }
$env:KONGCTL_DEFAULT_KONNECT_PAT = $pat

$omUrl = if ($env:OPENMETER_URL) { $env:OPENMETER_URL } else { 'https://us.api.konghq.com/v3/openmeter' }
$omUrl = $omUrl.TrimEnd('/')
$BASE = [regex]::Replace($omUrl, '(https?://[^/]+).*', '$1')
$PREFIX = [regex]::Replace($omUrl, 'https?://[^/]+', '')
if (-not $PREFIX) { $PREFIX = '/v3/openmeter' }

$catalogObj = Get-Content -Raw $Catalog | ConvertFrom-Json

# --- kongctl api helpers ---------------------------------------------------
function Kapi-Get($path) {
  $out = & kongctl api get "$PREFIX$path" --base-url $BASE -o json 2>$null
  if ($LASTEXITCODE -ne 0) { return $null }
  return ($out | ConvertFrom-Json)
}
function Kapi-Send($method, $path, $bodyJson) {
  $out = $bodyJson | & kongctl api $method "$PREFIX$path" --base-url $BASE -o json -f -
  if ($LASTEXITCODE -ne 0) { throw "kongctl api $method $path failed" }
  if ($out) { return ($out | ConvertFrom-Json) } else { return $null }
}
function Items($resp) {
  if ($null -eq $resp) { return @() }
  if ($resp.PSObject.Properties.Name -contains 'data') { return @($resp.data) }
  return @($resp)
}

# --- catalog ---------------------------------------------------------------
function Ensure-Meters {
  $existing = (Items (Kapi-Get '/meters')).key
  foreach ($m in $catalogObj.meters) {
    if ($existing -contains $m.key) { Info "meter   $($m.key) - exists"; continue }
    $body = [ordered]@{ name = $m.name; key = $m.key; description = $m.description; event_type = $m.event_type; aggregation = $m.aggregation }
    if ($m.value_property) { $body.value_property = $m.value_property }
    if ($m.dimensions) { $body.dimensions = $m.dimensions }
    Kapi-Send 'post' '/meters' ($body | ConvertTo-Json -Depth 6 -Compress) | Out-Null
    Info "meter   $($m.key) - created"
  }
}
function Ensure-Features {
  $existing = (Items (Kapi-Get '/features')).key
  foreach ($f in $catalogObj.features) {
    if ($existing -contains $f.key) { Info "feature $($f.key) - exists"; continue }
    $body = [ordered]@{ key = $f.key; name = $f.name; meter_slug = $f.meter_slug }
    Kapi-Send 'post' '/features' ($body | ConvertTo-Json -Compress) | Out-Null
    Info "feature $($f.key) - created"
  }
}
function Verify-Plan {
  $planKey = $catalogObj.plan_key
  if (-not $planKey) { Info 'plan    - none configured'; return }
  $found = Items (Kapi-Get '/plans') | Where-Object { $_.key -eq $planKey }
  if ($found) { Info "plan    $planKey - present" }
  else { Warn "plan    $planKey - NOT found; create it in Konnect before subscribing customers" }
}
function Invoke-Catalog {
  Info "== catalog ($BASE$PREFIX) =="
  Ensure-Meters; Ensure-Features; Verify-Plan
}

# --- customer --------------------------------------------------------------
function Ensure-Customer($clientId, $externalUserId, $display, $subscribe) {
  if (-not $clientId -or -not $externalUserId) { Die 'customer requires <client_id> <external_user_id>' }
  # CloudEvent subject = compound client_id:external_user_id = customer key = its only
  # subject_key. OpenMeter forbids changing subject_keys on subscribed customers, so we
  # set it at creation and never mutate it.
  $compound = "$clientId`:$externalUserId"
  if (-not $display) { $display = $compound }

  $cust = Items (Kapi-Get '/customers') | Where-Object { $_.key -eq $compound } | Select-Object -First 1
  if (-not $cust) {
    $body = [ordered]@{ key = $compound; name = $display; usage_attribution = @{ subject_keys = @($compound) } }
    $created = Kapi-Send 'post' '/customers' ($body | ConvertTo-Json -Depth 5 -Compress)
    $id = $created.id
    Info "customer $compound - created (subject: $compound)"
  }
  else {
    $id = $cust.id
    if ($cust.usage_attribution.subject_keys -contains $compound) { Info "customer $compound - up to date" }
    else {
      Warn "customer $compound exists but its subject_keys do not include '$compound'"
      Warn "  (OpenMeter blocks subject_key changes on subscribed customers - reconcile manually)"
    }
  }
  if ($subscribe) { Ensure-Subscription $id $compound }
}

function Ensure-Subscription($customerId, $label) {
  $planKey = $catalogObj.plan_key
  if (-not $planKey) { Warn 'no plan_key in catalog; skipping subscription'; return }
  $existing = Items (Kapi-Get "/subscriptions?customer_id=$customerId") | Where-Object { $_.customer_id -eq $customerId }
  if ($existing) { Info "sub      $label - exists"; return }
  $now = (Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ')
  $body = [ordered]@{ customer_id = $customerId; plan_key = $planKey; active_from = $now }
  try {
    Kapi-Send 'post' '/subscriptions' ($body | ConvertTo-Json -Compress) | Out-Null
    Info "sub      $label - created on $planKey"
  }
  catch { Warn "sub      $label - could not create subscription on $planKey (create manually if needed)" }
}

# --- dispatch --------------------------------------------------------------
switch ($Command) {
  'catalog' { Invoke-Catalog }
  'customer' { Ensure-Customer $ClientId $ExternalUserId $DisplayName $Subscribe.IsPresent }
  'all' { Invoke-Catalog; Ensure-Customer $ClientId $ExternalUserId $DisplayName $Subscribe.IsPresent }
}
Info 'done.'
