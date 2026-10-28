resource "container" "a" {
  command = ["a"]

  depends_on = [resource.container.b]
}

resource "container" "b" {
  command = resource.container.a.command
}
