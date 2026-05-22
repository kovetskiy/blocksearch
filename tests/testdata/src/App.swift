// Swift: searching the function name returns the whole func, braces
// included.
import Foundation

struct Calculator {
    let taxRate: Double = 0.20

    func computeTotal(subtotal: Double) -> Double {
        let tax = subtotal * taxRate
        return subtotal + tax
    }
}
