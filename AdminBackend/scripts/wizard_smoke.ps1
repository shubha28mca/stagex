$ErrorActionPreference = 'Stop'
$admin = 'http://localhost:8081'
$client = 'http://localhost:8080'

$ev = (Invoke-RestMethod -Method Post "$admin/admin/auth/login" -ContentType application/json -Body (@{email='event@stagex.test';password='Event@12345'} | ConvertTo-Json)).data
$EH = @{ Authorization = "Bearer $($ev.token)" }

$judges = (Invoke-RestMethod "$admin/admin/event/ref/judges" -Headers $EH).data
Write-Host "verified judges available: $($judges.Count)"

$start = (Get-Date).AddDays(40).ToString('yyyy-MM-dd')
$end = (Get-Date).AddDays(41).ToString('yyyy-MM-dd')
$body = @{
  name = 'Wizard Fest'; tagline = 'built via multi-step wizard'; city = 'Mumbai'; mode = 'onstage';
  coverGradient = 'pink'; startDate = $start; endDate = $end; fee = 400; slotsTotal = 150; rounds = 2;
  roundsDetail = @(@{name='Preliminary';description='Open round'}, @{name='Grand Finale';description='Top 10'});
  rubric = @(@{criterion='Technique';weight=60}, @{criterion='Expression';weight=40});
  judgeIds = @($judges[0].id)
} | ConvertTo-Json -Depth 6

$created = (Invoke-RestMethod -Method Post "$admin/admin/event/events" -Headers $EH -ContentType application/json -Body $body).data
Write-Host "created event id=$($created.id) rounds=$($created.roundsDetail.Count) rubric=$($created.rubric.Count) judges=$($created.judgeIds.Count)"

# add a category so the detail page shows categories & fees
$refCat = (Invoke-RestMethod "$admin/admin/event/ref/categories" -Headers $EH).data[0]
$refBand = (Invoke-RestMethod "$admin/admin/event/ref/age-bands" -Headers $EH).data[0]
Invoke-RestMethod -Method Post "$admin/admin/event/events/$($created.id)/categories" -Headers $EH -ContentType application/json -Body (@{categoryId=$refCat.id;ageBandId=$refBand.id;participationType='solo';fee=400} | ConvertTo-Json) | Out-Null

Invoke-RestMethod -Method Post "$admin/admin/event/events/$($created.id)/publish" -Headers $EH | Out-Null
Write-Host "published"

# Client detail endpoint should now expose the rich detail
$detail = (Invoke-RestMethod "$client/api/events/$($created.id)").data
Write-Host "CLIENT detail: rounds=$($detail.roundsDetail.Count) rubric=$($detail.rubric.Count) judges=$($detail.judges.Count) categories=$($detail.categories.Count)"
Write-Host "  round1=$($detail.roundsDetail[0].name) | rubric1=$($detail.rubric[0].criterion) $($detail.rubric[0].weight)% | judge1=$($detail.judges[0])"

# Verify it also shows on the public discover list (card)
$found = (Invoke-RestMethod "$client/api/events?city=Mumbai").data | Where-Object { $_.id -eq $created.id }
Write-Host "discover shows it: $([bool]$found)"

# cleanup
Invoke-RestMethod -Method Delete "$admin/admin/event/events/$($created.id)" -Headers $EH | Out-Null
Write-Host "WIZARD/DETAIL SMOKE PASSED"
