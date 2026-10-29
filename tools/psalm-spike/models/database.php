<?php

namespace Drupal\Core\Database;

abstract class Connection
{
    /** @psalm-taint-sink sql $query
     * @psalm-taint-specialize */
    public function query($query, array $args = [], $options = [])
    {
    }
}
