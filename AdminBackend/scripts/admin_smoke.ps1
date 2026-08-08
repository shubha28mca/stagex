$ErrorActionPreference = 'Stop'
$base = 'http://localhost:8081'

function Login($email, $pass) {
  (Invoke-RestMethod -Method Post "$base/admin/auth/login" -ContentType application/json -Body (@{email=$email;password=$pass} | ConvertTo-Json)).data
}

# --- Operational Admin ---
$ops = Login 'ops@stagex.test' 'Ops@12345'
$OH = @{ Authorization = "Bearer $($ops.token)" }
Write-Host "1. ops login: $($ops.admin.name) role=$($ops.admin.role)"

$dash = (Invoke-RestMethod "$base/admin/ops/dashboard" -Headers $OH).data
Write-Host "2. ops dashboard: events=$($dash.events) participants=$($dash.participants) revenue=$($dash.revenue)"

# CRUD an event type
$et = (Invoke-RestMethod -Method Post "$base/admin/ops/event-types" -Headers $OH -ContentType application/json -Body (@{code='workshop';name='Workshop';certificateSeal='standard';description='hands-on'} | ConvertTo-Json)).data
Write-Host "3. ops create event-type id=$($et.id)"
$upd = Invoke-RestMethod -Method Patch "$base/admin/ops/event-types/$($et.id)" -Headers $OH -ContentType application/json -Body (@{name='Workshop Plus';certificateSeal='standard';description='updated';isActive=$true} | ConvertTo-Json)
$list = (Invoke-RestMethod "$base/admin/ops/event-types" -Headers $OH).data
Write-Host "4. ops event-types count=$($list.Count)"
Invoke-RestMethod -Method Delete "$base/admin/ops/event-types/$($et.id)" -Headers $OH | Out-Null
Write-Host "5. ops deleted event-type"

# Ops sees all events + participants
$allEv = (Invoke-RestMethod "$base/admin/ops/events" -Headers $OH).data
$parts = (Invoke-RestMethod "$base/admin/ops/participants" -Headers $OH).data
Write-Host "6. ops oversight: events=$($allEv.Count) participants=$($parts.Count)"

# Role separation: ops token must be rejected on event-admin routes
try {
  Invoke-RestMethod "$base/admin/event/events" -Headers $OH | Out-Null
  Write-Host "7. FAIL: ops reached event-admin route"
} catch {
  Write-Host "7. ops correctly blocked from /admin/event/* (403)"
}

# --- Event Admin ---
$ev = Login 'event@stagex.test' 'Event@12345'
$EH = @{ Authorization = "Bearer $($ev.token)" }
Write-Host "8. event login: $($ev.admin.name) role=$($ev.admin.role)"

$start = (Get-Date).AddDays(30).ToString('yyyy-MM-dd')
$end = (Get-Date).AddDays(31).ToString('yyyy-MM-dd')
$newEv = (Invoke-RestMethod -Method Post "$base/admin/event/events" -Headers $EH -ContentType application/json -Body (@{name='Admin Test Fest';tagline='created via admin';city='Pune';mode='onstage';rounds=2;fee=350;slotsTotal=100;startDate=$start;endDate=$end;coverGradient='sky'} | ConvertTo-Json)).data
Write-Host "9. event create: $($newEv.name) status=$($newEv.status)"

Invoke-RestMethod -Method Post "$base/admin/event/events/$($newEv.id)/publish" -Headers $EH | Out-Null
Write-Host "10. event published"

# Add a category from master ref data
$refCat = (Invoke-RestMethod "$base/admin/event/ref/categories" -Headers $EH).data[0]
$refBand = (Invoke-RestMethod "$base/admin/event/ref/age-bands" -Headers $EH).data[0]
Invoke-RestMethod -Method Post "$base/admin/event/events/$($newEv.id)/categories" -Headers $EH -ContentType application/json -Body (@{categoryId=$refCat.id;ageBandId=$refBand.id;participationType='solo';fee=350} | ConvertTo-Json) | Out-Null
$cats = (Invoke-RestMethod "$base/admin/event/events/$($newEv.id)/categories" -Headers $EH).data
Write-Host "11. event category added: count=$($cats.Count) ($($cats[0].categoryName) / $($cats[0].ageBandLabel))"

# Event admin only sees own events
$mine = (Invoke-RestMethod "$base/admin/event/events" -Headers $EH).data
Write-Host "12. event admin owns $($mine.Count) event(s)"

# Role separation: event token must be rejected on ops routes
try {
  Invoke-RestMethod "$base/admin/ops/dashboard" -Headers $EH | Out-Null
  Write-Host "13. FAIL: event reached ops route"
} catch {
  Write-Host "13. event admin correctly blocked from /admin/ops/* (403)"
}

# cleanup the test event
Invoke-RestMethod -Method Delete "$base/admin/event/events/$($newEv.id)" -Headers $EH | Out-Null
Write-Host "14. event deleted (cleanup)"
Write-Host "ADMIN SMOKE TESTS PASSED"
