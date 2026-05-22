// Scala: searching the method name returns the whole def.
package billing

object Calculator {
  def computeTotal(subtotal: Double): Double = {
    val tax = subtotal * 0.20
    subtotal + tax
  }
}
