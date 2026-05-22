/* C: K&R braces, 4-space, preprocessor. */

#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#define MAX_LINE 1024

typedef struct node {
    char *value;
    struct node *next;
} node_t;

static node_t *prepend(node_t *head, const char *value) {
    node_t *n = malloc(sizeof(node_t));
    if (n == NULL) {
        return head;
    }
    n->value = strdup(value);
    n->next = head;
    return n;
}

static void free_list(node_t *head) {
    while (head != NULL) {
        node_t *tmp = head->next;
        free(head->value);
        free(head);
        head = tmp;
    }
}

int main(int argc, char **argv) {
    node_t *head = NULL;
    for (int i = 1; i < argc; i++) {
        if (strlen(argv[i]) < MAX_LINE) {
            head = prepend(head, argv[i]);
        }
    }

    for (node_t *cur = head; cur != NULL; cur = cur->next) {
        printf("%s\n", cur->value);
    }

    free_list(head);
    return 0;
}
