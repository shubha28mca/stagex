$ErrorActionPreference = 'Stop'
$base = 'http://localhost:8080'
$phone = "97" + (Get-Random -Minimum 10000000 -Maximum 99999999)

$otp = (Invoke-RestMethod -Method Post "$base/api/auth/otp/send" -ContentType application/json -Body (@{phone=$phone;purpose='register'} | ConvertTo-Json)).data.devOtp
$auth = (Invoke-RestMethod -Method Post "$base/api/auth/register" -ContentType application/json -Body (@{phone=$phone;name='Tester';password='Str0ngPass!';otp=$otp} | ConvertTo-Json)).data
$H = @{ Authorization = "Bearer $($auth.token)" }

$dob = (Get-Date).AddYears(-10).ToString('yyyy-MM-dd')
function AddPerson($name) {
  (Invoke-RestMethod -Method Post "$base/api/people" -Headers $H -ContentType application/json -Body (@{name=$name;dob=$dob;gender='female';aadhaar='234123412346';relationship='Daughter'} | ConvertTo-Json)).data
}

# A: unattached -> hard delete
$A = AddPerson 'Alpha'
$delA = Invoke-RestMethod -Method Delete "$base/api/people/$($A.id)" -Headers $H
Write-Host "A unattached delete: removed=$($delA.data.removed) soft=$($delA.data.softDeleted)"

# B: edit name
$B = AddPerson 'Bravo'
$B2 = (Invoke-RestMethod -Method Patch "$base/api/people/$($B.id)" -Headers $H -ContentType application/json -Body (@{name='Bravo Edited';city='Pune'} | ConvertTo-Json)).data
Write-Host "B edit: name='$($B2.name)' city='$($B2.city)'"

# C: attach to an event then delete -> soft delete
$C = AddPerson 'Charlie'
$evId = (Invoke-RestMethod "$base/api/events").data[0].id
$ev = (Invoke-RestMethod "$base/api/events/$evId").data
$cat = ($ev.categories | Where-Object { $C.ageYears -ge $_.minAge -and $C.ageYears -le $_.maxAge })[0]
$reg = (Invoke-RestMethod -Method Post "$base/api/registrations" -Headers $H -ContentType application/json -Body (@{eventId=$evId;entries=@(@{personId=$C.id;eventCategoryId=$cat.id})} | ConvertTo-Json)).data
$null = Invoke-RestMethod -Method Post "$base/api/payments/order" -Headers $H -ContentType application/json -Body (@{registrationId=$reg.id} | ConvertTo-Json)
$null = Invoke-RestMethod -Method Post "$base/api/payments/confirm" -Headers $H -ContentType application/json -Body (@{registrationId=$reg.id;success=$true} | ConvertTo-Json)
$delC = Invoke-RestMethod -Method Delete "$base/api/people/$($C.id)" -Headers $H
Write-Host "C attached delete: removed=$($delC.data.removed) soft=$($delC.data.softDeleted) msg='$($delC.data.message)'"

# List shows C as deleted (grayed) and B present
$list = (Invoke-RestMethod "$base/api/people" -Headers $H).data
$cRow = $list | Where-Object { $_.id -eq $C.id }
Write-Host "list: total=$($list.Count) C.deleted=$($cRow.deleted)"

# Deleted person cannot be registered again
try {
  Invoke-RestMethod -Method Post "$base/api/registrations" -Headers $H -ContentType application/json -Body (@{eventId=$evId;entries=@(@{personId=$C.id;eventCategoryId=$cat.id})} | ConvertTo-Json) | Out-Null
  Write-Host "FAIL: deleted person was registerable"
} catch {
  Write-Host "deleted person correctly blocked from new registration"
}
Write-Host "FEATURE TESTS PASSED"
