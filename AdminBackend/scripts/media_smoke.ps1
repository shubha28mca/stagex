$ErrorActionPreference = 'Stop'
$admin = 'http://localhost:8081'
$client = 'http://localhost:8080'

$ev = (Invoke-RestMethod -Method Post "$admin/admin/auth/login" -ContentType application/json -Body (@{email='event@stagex.test';password='Event@12345'} | ConvertTo-Json)).data
$EH = @{ Authorization = "Bearer $($ev.token)" }
(Invoke-RestMethod "$admin/admin/event/events" -Headers $EH).data | Where-Object { $_.name -eq 'Media Test' } | ForEach-Object {
  Invoke-RestMethod -Method Delete "$admin/admin/event/events/$($_.id)" -Headers $EH | Out-Null
}
$start = (Get-Date).AddDays(90).ToString('yyyy-MM-dd'); $end = (Get-Date).AddDays(91).ToString('yyyy-MM-dd')
$e = (Invoke-RestMethod -Method Post "$admin/admin/event/events" -Headers $EH -ContentType application/json -Body (@{name='Media Test';city='Mumbai';mode='onstage';startDate=$start;endDate=$end;fee=250;slotsTotal=50;rounds=1} | ConvertTo-Json)).data
$band = (Invoke-RestMethod "$admin/admin/event/ref/age-bands" -Headers $EH).data | Where-Object { $_.label -like '*9 to 12*' } | Select-Object -First 1
$cat = (Invoke-RestMethod "$admin/admin/event/ref/categories" -Headers $EH).data | Select-Object -First 1
Invoke-RestMethod -Method Post "$admin/admin/event/events/$($e.id)/categories" -Headers $EH -ContentType application/json -Body (@{categoryId=$cat.id;ageBandId=$band.id;participationType='solo';fee=250} | ConvertTo-Json) | Out-Null
Invoke-RestMethod -Method Post "$admin/admin/event/events/$($e.id)/publish" -Headers $EH | Out-Null
$ec = (Invoke-RestMethod "$admin/admin/event/events/$($e.id)/categories" -Headers $EH).data[0]
$phone = "92" + (Get-Random -Minimum 10000000 -Maximum 99999999)
$dob = (Get-Date).AddYears(-11).ToString('yyyy-MM-dd')
$off = (Invoke-RestMethod -Method Post "$admin/admin/event/events/$($e.id)/participants/offline" -Headers $EH -ContentType application/json -Body (@{name='Media Kid';dob=$dob;gender='male';phone=$phone;eventCategoryId=$ec.id} | ConvertTo-Json)).data
Write-Host "event + participant ready"

# 1) Upload a photo via multipart (curl.exe)
$img = "$env:TEMP\stagex_test.png"
[IO.File]::WriteAllBytes($img, [Convert]::FromBase64String('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAAC0lEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=='))
$resp = curl.exe -s -X POST -H "Authorization: Bearer $($ev.token)" -F "file=@$img" -F "kind=photo" "$admin/admin/event/events/$($e.id)/media"
$media = ($resp | ConvertFrom-Json).data
Write-Host "1. uploaded media id=$($media.id) url=$($media.url)"

# 2) Media list
$list = (Invoke-RestMethod "$admin/admin/event/events/$($e.id)/media" -Headers $EH).data
Write-Host "2. media list count=$($list.Count)"

# 3) Media is publicly served
$pub = Invoke-WebRequest $media.url -UseBasicParsing
Write-Host "3. public media fetch: status=$($pub.StatusCode) type=$($pub.Headers['Content-Type']) bytes=$($pub.RawContentLength)"

# 4) Issue a certificate so winners/certs show for the participant
$part = (Invoke-RestMethod "$admin/admin/event/participants" -Headers $EH).data | Where-Object { $_.entryCode -eq $off.entryCode } | Select-Object -First 1
Invoke-RestMethod -Method Post "$admin/admin/event/events/$($e.id)/certificates" -Headers $EH -ContentType application/json -Body (@{personId=$part.personId;position='gold'} | ConvertTo-Json) | Out-Null
Write-Host "4. certificate issued (gold)"

# 5) Participant sees media, winners, certificates on My Events
$otp = (Invoke-RestMethod -Method Post "$client/api/auth/otp/send" -ContentType application/json -Body (@{phone=$phone;purpose='login'} | ConvertTo-Json)).data.devOtp
$pa = (Invoke-RestMethod -Method Post "$client/api/auth/login" -ContentType application/json -Body (@{phone=$phone;otp=$otp} | ConvertTo-Json)).data
$PH = @{ Authorization = "Bearer $($pa.token)" }
$myev = (Invoke-RestMethod "$client/api/my/events" -Headers $PH).data | Where-Object { $_.eventId -eq $e.id } | Select-Object -First 1
Write-Host "5. participant My Events -> media=$($myev.media.Count) winners=$($myev.winners.Count) certificates=$($myev.certificates.Count)"

# cleanup
Invoke-RestMethod -Method Delete "$admin/admin/event/events/$($e.id)" -Headers $EH | Out-Null
Remove-Item $img -ErrorAction SilentlyContinue
Write-Host "MEDIA/VIEW SMOKE PASSED"
