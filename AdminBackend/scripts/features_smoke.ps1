$ErrorActionPreference = 'Stop'
$admin = 'http://localhost:8081'
$client = 'http://localhost:8080'

# --- Event Admin setup ---
$ev = (Invoke-RestMethod -Method Post "$admin/admin/auth/login" -ContentType application/json -Body (@{email='event@stagex.test';password='Event@12345'} | ConvertTo-Json)).data
$EH = @{ Authorization = "Bearer $($ev.token)" }

# Remove any stale test events from earlier runs.
(Invoke-RestMethod "$admin/admin/event/events" -Headers $EH).data | Where-Object { $_.name -eq 'Feature Fest' } | ForEach-Object {
  Invoke-RestMethod -Method Delete "$admin/admin/event/events/$($_.id)" -Headers $EH | Out-Null
}

$start = (Get-Date).AddDays(60).ToString('yyyy-MM-dd'); $end = (Get-Date).AddDays(61).ToString('yyyy-MM-dd')
$e = (Invoke-RestMethod -Method Post "$admin/admin/event/events" -Headers $EH -ContentType application/json -Body (@{name='Feature Fest';city='Mumbai';mode='onstage';startDate=$start;endDate=$end;fee=250;slotsTotal=50;rounds=1} | ConvertTo-Json)).data
$band = (Invoke-RestMethod "$admin/admin/event/ref/age-bands" -Headers $EH).data | Where-Object { $_.label -like '*9 to 12*' } | Select-Object -First 1
$cat = (Invoke-RestMethod "$admin/admin/event/ref/categories" -Headers $EH).data | Select-Object -First 1
Invoke-RestMethod -Method Post "$admin/admin/event/events/$($e.id)/categories" -Headers $EH -ContentType application/json -Body (@{categoryId=$cat.id;ageBandId=$band.id;participationType='solo';fee=250} | ConvertTo-Json) | Out-Null
Invoke-RestMethod -Method Post "$admin/admin/event/events/$($e.id)/publish" -Headers $EH | Out-Null
$ec = (Invoke-RestMethod "$admin/admin/event/events/$($e.id)/categories" -Headers $EH).data[0]
Write-Host "event ready id=$($e.id)"

# 1) Ad-hoc participant with offline payment
$phone = "95" + (Get-Random -Minimum 10000000 -Maximum 99999999)
$dob = (Get-Date).AddYears(-11).ToString('yyyy-MM-dd')
$off = (Invoke-RestMethod -Method Post "$admin/admin/event/events/$($e.id)/participants/offline" -Headers $EH -ContentType application/json -Body (@{name='Walkin Kid';dob=$dob;gender='male';aadhaar='234123412346';phone=$phone;eventCategoryId=$ec.id} | ConvertTo-Json)).data
Write-Host "1. offline participant added: entry=$($off.entryCode)"

# 2) Crew
Invoke-RestMethod -Method Post "$admin/admin/event/events/$($e.id)/crew" -Headers $EH -ContentType application/json -Body (@{name='Sam Volunteer';role='Registration';contact='sam@x.test'} | ConvertTo-Json) | Out-Null
$crew = (Invoke-RestMethod "$admin/admin/event/events/$($e.id)/crew" -Headers $EH).data
Write-Host "2. crew count=$($crew.Count) ($($crew[0].name)/$($crew[0].role))"

# 3) Notifications: config + broadcast to all
Invoke-RestMethod -Method Put "$admin/admin/event/events/$($e.id)/notifications/config" -Headers $EH -ContentType application/json -Body (@{registration_confirmed=@{inApp=$true;email=$true}} | ConvertTo-Json) | Out-Null
Invoke-RestMethod -Method Post "$admin/admin/event/events/$($e.id)/notifications" -Headers $EH -ContentType application/json -Body (@{audience='all';title='Welcome to Feature Fest';message='Reporting time is 9 AM at Gate 2.'} | ConvertTo-Json) | Out-Null
Write-Host "3. notification config saved + broadcast sent"

# 4) Winner / certificate for the walk-in
$part = (Invoke-RestMethod "$admin/admin/event/participants" -Headers $EH).data | Where-Object { $_.entryCode -eq $off.entryCode } | Select-Object -First 1
$img = 'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=='
Invoke-RestMethod -Method Post "$admin/admin/event/events/$($e.id)/certificates" -Headers $EH -ContentType application/json -Body (@{personId=$part.personId;position='gold';imageUrl=$img} | ConvertTo-Json) | Out-Null
$certs = (Invoke-RestMethod "$admin/admin/event/events/$($e.id)/certificates" -Headers $EH).data
Write-Host "4. certificate issued: $($certs[0].personName) position=$($certs[0].position)"

# 5) Report export CSV + PDF
$csv = Invoke-WebRequest "$admin/admin/event/events/$($e.id)/report?format=csv" -Headers $EH -UseBasicParsing
$pdf = Invoke-WebRequest "$admin/admin/event/events/$($e.id)/report?format=pdf" -Headers $EH -UseBasicParsing
$pdfHead = [System.Text.Encoding]::ASCII.GetString($pdf.Content[0..3])
Write-Host "5. CSV type=$($csv.Headers['Content-Type']) bytes=$($csv.RawContentLength) | PDF head=$pdfHead bytes=$($pdf.RawContentLength)"

# --- Participant side: the walk-in's family logs in via OTP and sees things ---
$otp = (Invoke-RestMethod -Method Post "$client/api/auth/otp/send" -ContentType application/json -Body (@{phone=$phone;purpose='login'} | ConvertTo-Json)).data.devOtp
$pa = (Invoke-RestMethod -Method Post "$client/api/auth/login" -ContentType application/json -Body (@{phone=$phone;otp=$otp} | ConvertTo-Json)).data
$PH = @{ Authorization = "Bearer $($pa.token)" }
$notes = (Invoke-RestMethod "$client/api/my/notifications" -Headers $PH).data
$mycerts = (Invoke-RestMethod "$client/api/my/certificates" -Headers $PH).data
Write-Host "6. participant sees notifications=$($notes.Count) ('$($notes[0].title)') certificates=$($mycerts.Count) (pos=$($mycerts[0].position), hasImage=$([bool]$mycerts[0].fileUrl))"

# cleanup
Invoke-RestMethod -Method Delete "$admin/admin/event/events/$($e.id)" -Headers $EH | Out-Null
Write-Host "EVENT ADMIN FEATURES SMOKE PASSED"
