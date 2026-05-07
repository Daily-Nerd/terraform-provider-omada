resource "omada_site" "example" {
  name      = "Branch-Office"
  region    = "United States"
  time_zone = "UTC"
  scenario  = "Office"
  type      = 0

  device_account_username = "admin"
  device_account_password = "secret-password"
}
