resource "container" "a" {
  command = ["a"]

  depends_on = ["resource.container.b"]
}

resource "container" "b" {
  command = ["b"]

  depends_on = ["resource.container.a"]
}
