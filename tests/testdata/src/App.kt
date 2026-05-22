// Kotlin: searching the function name returns the whole fun, braces
// included.
package billing

class ReceiptPrinter {
    fun format(total: Double): String {
        val tax = total * 0.20
        return "total=%.2f".format(total + tax)
    }
}
