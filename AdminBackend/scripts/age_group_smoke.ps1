$ErrorActionPreference = 'Stop'
$admin = 'http://localhost:8081'
$client = 'http://localhost:8080'

# --- Event Admin: create an event and give it an age group ---
$ev = (Invoke-RestMethod -Method Post "$admin/admin/auth/login" -ContentType application/json -Body (@{email='event@stagex.test';password='Event@12345'} | ConvertTo-Json)).data
$EH = @{ Authorization = "Bearer $($ev.token)" }

$start = (Get-Date).AddDays(50).ToString('yyyy-MM-dd')
$end = (Get-Date).AddDays(51).ToString('yyyy-MM-dd')
$created = (Invoke-RestMethod -Method Post "$admin/admin/event/events" -Headers $EH -ContentType application/json -Body (@{name='Age Group Fest';city='Mumbai';mode='onstage';startDate=$start;endDate=$end;fee=300;slotsTotal=100;rounds=1} | ConvertTo-Json)).data
Write-Host "created event id=$($created.id)"

$band = (Invoke-RestMethod "$admin/admin/event/ref/age-bands" -Headers $EH).data | Where-Object { $_.label -like '*9 to 12*' } | Select-Object -First 1
$cat = (Invoke-RestMethod "$admin/admin/event/ref/categories" -Headers $EH).data | Select-Object -First 1
Write-Host "chosen age group: $($cat.label) / $($band.label)"
Invoke-RestMethod -Method Post "$admin/admin/event/events/$($created.id)/categories" -Headers $EH -ContentType application/json -Body (@{categoryId=$cat.id;ageBandId=$band.id;participationType='solo';fee=300} | ConvertTo-Json) | Out-Null
Invoke-RestMethod -Method Post "$admin/admin/event/events/$($created.id)/publish" -Headers $EH | Out-Null
Write-Host "age group added + published"

# --- Participant: a 10-year-old should now match and register ---
$phone = "96" + (Get-Random -Minimum 10000000 -Maximum 99999999)
$otp = (Invoke-RestMethod -Method Post "$client/api/auth/otp/send" -ContentType application/json -Body (@{phone=$phone;purpose='register'} | ConvertTo-Json)).data.devOtp
$auth = (Invoke-RestMethod -Method Post "$client/api/auth/register" -ContentType application/json -Body (@{phone=$phone;name='Test';password='Str0ngPass!';otp=$otp} | ConvertTo-Json)).data
$PH = @{ Authorization = "Bearer $($auth.token)" }
$dob = (Get-Date).AddYears(-11).ToString('yyyy-MM-dd')
$person = (Invoke-RestMethod -Method Post "$client/api/people" -Headers $PH -ContentType application/json -Body (@{name='Kid';dob=$dob;gender='male';aadhaar='234123412346';relationship='Son'} | ConvertTo-Json)).data
Write-Host "participant age=$($person.ageYears)"

$detail = (Invoke-RestMethod "$client/api/events/$($created.id)").data
$ec = $detail.categories | Where-Object { $person.ageYears -ge $_.minAge -and $person.ageYears -le $_.maxAge } | Select-Object -First 1
if (-not $ec) { throw "FAIL: no matching age group for the participant" }
Write-Host "matched category: $($ec.categoryName) ($($ec.ageBandLabel)) minAge=$($ec.minAge) maxAge=$($ec.maxAge)"

$reg = (Invoke-RestMethod -Method Post "$client/api/registrations" -Headers $PH -ContentType application/json -Body (@{eventId=$created.id;entries=@(@{personId=$person.id;eventCategoryId=$ec.id})} | ConvertTo-Json)).data
Write-Host "registration created: entry=$($reg.entries[0].entryCode) total=$($reg.total)"

# cleanup
Invoke-RestMethod -Method Delete "$admin/admin/event/events/$($created.id)" -Headers $EH | Out-Null
Write-Host "AGE GROUP SMOKE PASSED"
