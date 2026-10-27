resource "network" "first" {
  subnet = "10.7.0.0/16"
}

resource "container" "second" {
  command = ["second"]

  depends_on = [resource.network.first]
}
