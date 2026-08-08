$ErrorActionPreference = 'Stop'
$admin = 'http://localhost:8081'

# --- Event with revenue (Event Admin + offline paid entry) ---
$ev = (Invoke-RestMethod -Method Post "$admin/admin/auth/login" -ContentType application/json -Body (@{email='event@stagex.test';password='Event@12345'} | ConvertTo-Json)).data
$EH = @{ Authorization = "Bearer $($ev.token)" }
(Invoke-RestMethod "$admin/admin/event/events" -Headers $EH).data | Where-Object { $_.name -eq 'VS Test' } | ForEach-Object {
  Invoke-RestMethod -Method Delete "$admin/admin/event/events/$($_.id)" -Headers $EH | Out-Null
}
$start = (Get-Date).AddDays(80).ToString('yyyy-MM-dd'); $end = (Get-Date).AddDays(81).ToString('yyyy-MM-dd')
$e = (Invoke-RestMethod -Method Post "$admin/admin/event/events" -Headers $EH -ContentType application/json -Body (@{name='VS Test';city='Mumbai';mode='onstage';startDate=$start;endDate=$end;fee=250;slotsTotal=50;rounds=1} | ConvertTo-Json)).data
$band = (Invoke-RestMethod "$admin/admin/event/ref/age-bands" -Headers $EH).data | Where-Object { $_.label -like '*9 to 12*' } | Select-Object -First 1
$cat = (Invoke-RestMethod "$admin/admin/event/ref/categories" -Headers $EH).data | Select-Object -First 1
Invoke-RestMethod -Method Post "$admin/admin/event/events/$($e.id)/categories" -Headers $EH -ContentType application/json -Body (@{categoryId=$cat.id;ageBandId=$band.id;participationType='solo';fee=250} | ConvertTo-Json) | Out-Null
Invoke-RestMethod -Method Post "$admin/admin/event/events/$($e.id)/publish" -Headers $EH | Out-Null
$ec = (Invoke-RestMethod "$admin/admin/event/events/$($e.id)/categories" -Headers $EH).data[0]
$phone = "93" + (Get-Random -Minimum 10000000 -Maximum 99999999)
$dob = (Get-Date).AddYears(-11).ToString('yyyy-MM-dd')
Invoke-RestMethod -Method Post "$admin/admin/event/events/$($e.id)/participants/offline" -Headers $EH -ContentType application/json -Body (@{name='Payer';dob=$dob;gender='male';phone=$phone;eventCategoryId=$ec.id} | ConvertTo-Json) | Out-Null
Write-Host "event ready (revenue 250)"

# --- Ops: vendors + sponsors ---
$op = (Invoke-RestMethod -Method Post "$admin/admin/auth/login" -ContentType application/json -Body (@{email='ops@stagex.test';password='Ops@12345'} | ConvertTo-Json)).data
$OH = @{ Authorization = "Bearer $($op.token)" }

$vendor = (Invoke-RestMethod -Method Post "$admin/admin/ops/vendors" -Headers $OH -ContentType application/json -Body (@{name='LED Wall Co';serviceType='AV';city='Mumbai';contact='led@x.test'} | ConvertTo-Json)).data
Write-Host "1. vendor created: $($vendor.name)"
$sponsor = (Invoke-RestMethod -Method Post "$admin/admin/ops/sponsors" -Headers $OH -ContentType application/json -Body (@{organisation='MegaBank';tier='platinum';contact='spon@x.test';committedAmount=100000;scholarshipSlots=10} | ConvertTo-Json)).data
Write-Host "2. sponsor created: $($sponsor.organisation)"

Invoke-RestMethod -Method Post "$admin/admin/ops/events/$($e.id)/vendors/assign" -Headers $OH -ContentType application/json -Body (@{vendorId=$vendor.id;cost=1000} | ConvertTo-Json) | Out-Null
Invoke-RestMethod -Method Post "$admin/admin/ops/events/$($e.id)/sponsors/assign" -Headers $OH -ContentType application/json -Body (@{sponsorId=$sponsor.id;cost=2000} | ConvertTo-Json) | Out-Null
$av = (Invoke-RestMethod "$admin/admin/ops/events/$($e.id)/vendors" -Headers $OH).data
$as = (Invoke-RestMethod "$admin/admin/ops/events/$($e.id)/sponsors" -Headers $OH).data
Write-Host "3. assigned vendor cost=$($av[0].cost) sponsor cost=$($as[0].cost)"

$pnl = (Invoke-RestMethod "$admin/admin/ops/events/$($e.id)/pnl" -Headers $OH).data
Write-Host "4. P&L: revenue=$($pnl.revenue) sponsorIncome=$($pnl.sponsorIncome) vendorIncome=$($pnl.vendorIncome) totalIncome=$($pnl.totalIncome) totalExpenses=$($pnl.totalExpenses) net=$($pnl.netPL) margin=$([math]::Round($pnl.margin,1))%"
$expected = 250 + 2000 + 1000
if ([math]::Round($pnl.netPL) -ne $expected) { throw "FAIL: expected net $expected got $($pnl.netPL)" }
Write-Host "   net matches expected $expected (income adds to profit)"

$csv = Invoke-WebRequest "$admin/admin/ops/events/$($e.id)/report?format=csv" -Headers $OH -UseBasicParsing
$hasV = $csv.Content -match 'LED Wall Co'; $hasS = $csv.Content -match 'MegaBank'
Write-Host "5. report includes vendor=$hasV sponsor=$hasS"

# cleanup
Invoke-RestMethod -Method Delete "$admin/admin/event/events/$($e.id)" -Headers $EH | Out-Null
Invoke-RestMethod -Method Delete "$admin/admin/ops/vendors/$($vendor.id)" -Headers $OH | Out-Null
Invoke-RestMethod -Method Delete "$admin/admin/ops/sponsors/$($sponsor.id)" -Headers $OH | Out-Null
Write-Host "VENDOR/SPONSOR SMOKE PASSED"
