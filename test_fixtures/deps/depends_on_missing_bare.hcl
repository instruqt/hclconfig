resource "container" "a" {
  command = ["a"]

  depends_on = [resource.network.missing]
}
