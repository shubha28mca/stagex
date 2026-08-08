$ErrorActionPreference = 'Stop'
$admin = 'http://localhost:8081'

# --- Set up an event with revenue (via Event Admin + offline paid entry) ---
$ev = (Invoke-RestMethod -Method Post "$admin/admin/auth/login" -ContentType application/json -Body (@{email='event@stagex.test';password='Event@12345'} | ConvertTo-Json)).data
$EH = @{ Authorization = "Bearer $($ev.token)" }
(Invoke-RestMethod "$admin/admin/event/events" -Headers $EH).data | Where-Object { $_.name -eq 'Ops Test' } | ForEach-Object {
  Invoke-RestMethod -Method Delete "$admin/admin/event/events/$($_.id)" -Headers $EH | Out-Null
}
$start = (Get-Date).AddDays(70).ToString('yyyy-MM-dd'); $end = (Get-Date).AddDays(71).ToString('yyyy-MM-dd')
$e = (Invoke-RestMethod -Method Post "$admin/admin/event/events" -Headers $EH -ContentType application/json -Body (@{name='Ops Test';city='Mumbai';mode='onstage';startDate=$start;endDate=$end;fee=250;slotsTotal=50;rounds=1} | ConvertTo-Json)).data
$band = (Invoke-RestMethod "$admin/admin/event/ref/age-bands" -Headers $EH).data | Where-Object { $_.label -like '*9 to 12*' } | Select-Object -First 1
$cat = (Invoke-RestMethod "$admin/admin/event/ref/categories" -Headers $EH).data | Select-Object -First 1
Invoke-RestMethod -Method Post "$admin/admin/event/events/$($e.id)/categories" -Headers $EH -ContentType application/json -Body (@{categoryId=$cat.id;ageBandId=$band.id;participationType='solo';fee=250} | ConvertTo-Json) | Out-Null
Invoke-RestMethod -Method Post "$admin/admin/event/events/$($e.id)/publish" -Headers $EH | Out-Null
$ec = (Invoke-RestMethod "$admin/admin/event/events/$($e.id)/categories" -Headers $EH).data[0]
$phone = "94" + (Get-Random -Minimum 10000000 -Maximum 99999999)
$dob = (Get-Date).AddYears(-11).ToString('yyyy-MM-dd')
Invoke-RestMethod -Method Post "$admin/admin/event/events/$($e.id)/participants/offline" -Headers $EH -ContentType application/json -Body (@{name='Paid Kid';dob=$dob;gender='male';phone=$phone;eventCategoryId=$ec.id} | ConvertTo-Json) | Out-Null
Write-Host "event ready with 1 paid entry (revenue 250)"

# --- Operations Admin ---
$op = (Invoke-RestMethod -Method Post "$admin/admin/auth/login" -ContentType application/json -Body (@{email='ops@stagex.test';password='Ops@12345'} | ConvertTo-Json)).data
$OH = @{ Authorization = "Bearer $($op.token)" }

# Hall price edit
$hall = (Invoke-RestMethod "$admin/admin/ops/halls" -Headers $OH).data | Select-Object -First 1
Invoke-RestMethod -Method Patch "$admin/admin/ops/halls/$($hall.id)" -Headers $OH -ContentType application/json -Body (@{name=$hall.name;city=$hall.city;capacity=$hall.capacity;baseRate=77777;leadName=$hall.leadName;leadContact=$hall.leadContact;isActive=$true} | ConvertTo-Json) | Out-Null
$hall2 = (Invoke-RestMethod "$admin/admin/ops/halls" -Headers $OH).data | Where-Object { $_.id -eq $hall.id }
Write-Host "1. hall price updated -> $($hall2.baseRate)"

# Crew pool with cost
$crew = (Invoke-RestMethod -Method Post "$admin/admin/ops/crew" -Headers $OH -ContentType application/json -Body (@{name='Lead A';role='Stage';cost=500;contact='lead@x.test'} | ConvertTo-Json)).data
Write-Host "2. crew pool member created cost=$($crew.cost)"

# Assign crew to event
Invoke-RestMethod -Method Post "$admin/admin/ops/events/$($e.id)/crew/assign" -Headers $OH -ContentType application/json -Body (@{crewId=$crew.id} | ConvertTo-Json) | Out-Null
$assigned = (Invoke-RestMethod "$admin/admin/ops/events/$($e.id)/crew" -Headers $OH).data
Write-Host "3. crew assigned to event: count=$($assigned.Count) cost=$($assigned[0].cost)"

# Additional expense with comment
Invoke-RestMethod -Method Post "$admin/admin/ops/events/$($e.id)/expenses" -Headers $OH -ContentType application/json -Body (@{amount=300;comment='Stage banner printing'} | ConvertTo-Json) | Out-Null
Write-Host "4. expense added (300, banner)"

# P&L
$pnl = (Invoke-RestMethod "$admin/admin/ops/events/$($e.id)/pnl" -Headers $OH).data
Write-Host "5. P&L: revenue=$($pnl.revenue) crew=$($pnl.crewCost) exp=$($pnl.expenses) hall=$($pnl.hallCost) total=$($pnl.totalExpenses) net=$($pnl.netPL) margin=$([math]::Round($pnl.margin,1))% participants=$($pnl.participants)"

# Reports
$csv = Invoke-WebRequest "$admin/admin/ops/events/$($e.id)/report?format=csv" -Headers $OH -UseBasicParsing
$pdf = Invoke-WebRequest "$admin/admin/ops/events/$($e.id)/report?format=pdf" -Headers $OH -UseBasicParsing
$pdfHead = [System.Text.Encoding]::ASCII.GetString($pdf.Content[0..3])
Write-Host "6. reports: CSV=$($csv.RawContentLength)b PDF head=$pdfHead $($pdf.RawContentLength)b"

# Archive (download + purge)
$arch = Invoke-WebRequest -Method Post "$admin/admin/ops/events/$($e.id)/archive" -Headers $OH -UseBasicParsing
Write-Host "7. archive: type=$($arch.Headers['Content-Type']) bytes=$($arch.RawContentLength)"
$gone = -not ((Invoke-RestMethod "$admin/admin/ops/events" -Headers $OH).data | Where-Object { $_.id -eq $e.id })
Write-Host "8. event purged after archive: $gone"

# cleanup crew pool member
Invoke-RestMethod -Method Delete "$admin/admin/ops/crew/$($crew.id)" -Headers $OH | Out-Null
Write-Host "OPS FEATURES SMOKE PASSED"
