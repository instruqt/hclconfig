resource "network" "main" {
  subnet = "10.0.5.0/24"
}

resource "container" "quoted" {
  command = ["a"]

  depends_on = ["resource.network.main"]
}

resource "container" "bare" {
  command = ["b"]

  depends_on = [resource.network.main]
}

resource "container" "mixed" {
  command = ["c"]

  depends_on = [
    resource.network.main,
    "resource.container.quoted",
  ]
}
