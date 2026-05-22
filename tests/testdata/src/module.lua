-- Lua: searching the function name returns the whole function...end.
local Billing = {}

function Billing.compute_total(subtotal)
    local tax = subtotal * 0.20
    return subtotal + tax
end

return Billing
