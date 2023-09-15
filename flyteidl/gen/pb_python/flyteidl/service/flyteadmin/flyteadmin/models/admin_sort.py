# coding: utf-8

"""
    flyteidl/service/admin.proto

    No description provided (generated by Swagger Codegen https://github.com/swagger-api/swagger-codegen)  # noqa: E501

    OpenAPI spec version: version not set
    
    Generated by: https://github.com/swagger-api/swagger-codegen.git
"""


import pprint
import re  # noqa: F401

import six

from flyteadmin.models.sort_direction import SortDirection  # noqa: F401,E501


class AdminSort(object):
    """NOTE: This class is auto generated by the swagger code generator program.

    Do not edit the class manually.
    """

    """
    Attributes:
      swagger_types (dict): The key is attribute name
                            and the value is attribute type.
      attribute_map (dict): The key is attribute name
                            and the value is json key in definition.
    """
    swagger_types = {
        'key': 'str',
        'direction': 'SortDirection'
    }

    attribute_map = {
        'key': 'key',
        'direction': 'direction'
    }

    def __init__(self, key=None, direction=None):  # noqa: E501
        """AdminSort - a model defined in Swagger"""  # noqa: E501

        self._key = None
        self._direction = None
        self.discriminator = None

        if key is not None:
            self.key = key
        if direction is not None:
            self.direction = direction

    @property
    def key(self):
        """Gets the key of this AdminSort.  # noqa: E501


        :return: The key of this AdminSort.  # noqa: E501
        :rtype: str
        """
        return self._key

    @key.setter
    def key(self, key):
        """Sets the key of this AdminSort.


        :param key: The key of this AdminSort.  # noqa: E501
        :type: str
        """

        self._key = key

    @property
    def direction(self):
        """Gets the direction of this AdminSort.  # noqa: E501


        :return: The direction of this AdminSort.  # noqa: E501
        :rtype: SortDirection
        """
        return self._direction

    @direction.setter
    def direction(self, direction):
        """Sets the direction of this AdminSort.


        :param direction: The direction of this AdminSort.  # noqa: E501
        :type: SortDirection
        """

        self._direction = direction

    def to_dict(self):
        """Returns the model properties as a dict"""
        result = {}

        for attr, _ in six.iteritems(self.swagger_types):
            value = getattr(self, attr)
            if isinstance(value, list):
                result[attr] = list(map(
                    lambda x: x.to_dict() if hasattr(x, "to_dict") else x,
                    value
                ))
            elif hasattr(value, "to_dict"):
                result[attr] = value.to_dict()
            elif isinstance(value, dict):
                result[attr] = dict(map(
                    lambda item: (item[0], item[1].to_dict())
                    if hasattr(item[1], "to_dict") else item,
                    value.items()
                ))
            else:
                result[attr] = value
        if issubclass(AdminSort, dict):
            for key, value in self.items():
                result[key] = value

        return result

    def to_str(self):
        """Returns the string representation of the model"""
        return pprint.pformat(self.to_dict())

    def __repr__(self):
        """For `print` and `pprint`"""
        return self.to_str()

    def __eq__(self, other):
        """Returns true if both objects are equal"""
        if not isinstance(other, AdminSort):
            return False

        return self.__dict__ == other.__dict__

    def __ne__(self, other):
        """Returns true if both objects are not equal"""
        return not self == other