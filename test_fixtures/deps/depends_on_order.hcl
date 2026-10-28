resource "network" "main" {
  subnet = "10.0.5.0/24"
}

resource "container" "bare" {
  command = ["bare"]

  depends_on = [resource.network.main]
}

resource "container" "quoted" {
  command = ["quoted"]

  depends_on = ["resource.network.main"]
}

resource "container" "mixed" {
  command = ["mixed"]

  depends_on = [
    resource.network.main,
    "module.first",
  ]
}

resource "container" "after_module" {
  command = ["after_module"]

  depends_on = [module.first]
}

module "first" {
  source = "../single"
}

module "bare" {
  source     = "../single"
  depends_on = [module.first]
}

module "quoted" {
  source     = "../single"
  depends_on = ["module.first"]
}

module "sibling" {
  source = "./sibling"
}
