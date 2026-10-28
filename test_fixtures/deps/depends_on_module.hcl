module "first" {
  source = "../single"

  variables = {
    cpu_resources = 1024
  }
}

module "quoted" {
  source     = "../single"
  depends_on = ["module.first"]
}

module "bare" {
  source     = "../single"
  depends_on = [module.first]

  variables = {
    cpu_resources = module.first.output.container_resources_cpu
  }
}
