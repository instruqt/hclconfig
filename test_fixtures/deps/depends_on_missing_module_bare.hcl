resource "container" "a" {
  command = ["a"]

  depends_on = [module.typo]
}
