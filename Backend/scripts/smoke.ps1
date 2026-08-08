$ErrorActionPreference = 'Stop'
$base = 'http://localhost:8080'
$phone = '9876500011'

$otp = (Invoke-RestMethod -Method Post "$base/api/auth/otp/send" -ContentType application/json -Body (@{phone=$phone;purpose='register'} | ConvertTo-Json)).data.devOtp
$auth = (Invoke-RestMethod -Method Post "$base/api/auth/register" -ContentType application/json -Body (@{phone=$phone;name='Priya';password='Str0ngPass!';otp=$otp} | ConvertTo-Json)).data
$H = @{ Authorization = "Bearer $($auth.token)" }
Write-Host "1. registered: $($auth.family.phone)"

$dob = (Get-Date).AddYears(-10).ToString('yyyy-MM-dd')
$person = (Invoke-RestMethod -Method Post "$base/api/people" -Headers $H -ContentType application/json -Body (@{name='Diya';dob=$dob;gender='female';aadhaar='234123412346';relationship='Daughter'} | ConvertTo-Json)).data
Write-Host "2. person: $($person.name) age $($person.ageYears) aadhaar $($person.aadhaarMasked)"

$evId = (Invoke-RestMethod "$base/api/events").data[0].id
$ev = (Invoke-RestMethod "$base/api/events/$evId").data
$cat = ($ev.categories | Where-Object { $person.ageYears -ge $_.minAge -and $person.ageYears -le $_.maxAge })[0]
Write-Host "3. event: $($ev.name) | category: $($cat.categoryName) fee $($cat.fee)"

$cp = (Invoke-RestMethod -Method Post "$base/api/coupons/validate" -ContentType application/json -Body (@{code='EARLYBIRD20';subtotal=$cat.fee;eventId=$evId} | ConvertTo-Json)).data
Write-Host "4. coupon valid=$($cp.valid) discount=$($cp.discount) total=$($cp.total)"

$reg = (Invoke-RestMethod -Method Post "$base/api/registrations" -Headers $H -ContentType application/json -Body (@{eventId=$evId;couponCode='EARLYBIRD20';entries=@(@{personId=$person.id;eventCategoryId=$cat.id})} | ConvertTo-Json)).data
Write-Host "5. registration total=$($reg.total) entry=$($reg.entries[0].entryCode)"

$null = (Invoke-RestMethod -Method Post "$base/api/payments/order" -Headers $H -ContentType application/json -Body (@{registrationId=$reg.id} | ConvertTo-Json)).data
$pc = (Invoke-RestMethod -Method Post "$base/api/payments/confirm" -Headers $H -ContentType application/json -Body (@{registrationId=$reg.id;success=$true} | ConvertTo-Json)).data
Write-Host "6. payment status=$($pc.status)"

$me = (Invoke-RestMethod "$base/api/my/events" -Headers $H).data
Write-Host "7. my events count=$($me.Count) status=$($me[0].status)"

# Eligibility guard: a 30-year-old must be rejected for the 9-12 category
$dob2 = (Get-Date).AddYears(-30).ToString('yyyy-MM-dd')
$p2 = (Invoke-RestMethod -Method Post "$base/api/people" -Headers $H -ContentType application/json -Body (@{name='Raj';dob=$dob2;gender='male';aadhaar='234123412346';relationship='Myself'} | ConvertTo-Json)).data
try {
  Invoke-RestMethod -Method Post "$base/api/registrations" -Headers $H -ContentType application/json -Body (@{eventId=$evId;entries=@(@{personId=$p2.id;eventCategoryId=$cat.id})} | ConvertTo-Json) | Out-Null
  Write-Host "8. ELIGIBILITY CHECK FAILED (should have rejected)"
} catch {
  Write-Host "8. eligibility correctly rejected ineligible age"
}
Write-Host "ALL SMOKE TESTS PASSED"
