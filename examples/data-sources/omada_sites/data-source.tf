data "omada_sites" "all" {}

output "sites" {
  value = data.omada_sites.all.sites
}
