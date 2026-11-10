resource "network" "on-prem" {
  subnet = "10.6.0.0/16"
}

resource "container" "a" {
  networkobj = resource.network["on-prem"]
}

resource "container" "b" {
  network {
    name = resource.container.a.meta.name
  }
}
